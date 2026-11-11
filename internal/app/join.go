package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/joincode"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// JoinService turns a join code into a device (design 0015): an
// administrator makes a code for a group, a laptop presents it with its
// serial number and model, and gets back its tag and its own credential.
//
// The code is the approval, so nothing here asks anyone. What it does decide
// is WHICH device the laptop becomes, and that is where the care goes: a
// serial number is not a secret, so a code must never be able to take over a
// device that already exists. A provisional record with the same serial is
// the same unfinished join and is reused; a working device with the same
// serial is refused, and only a code made for that device (Re-enrol) can
// give it a new credential.
type JoinService struct {
	codes  ports.JoinCodeStore
	cfg    *ConfigService
	creds  *DeviceCredentials
	clock  ports.Clock
	tenant string
	key    []byte
	rand   io.Reader
}

// NewJoinService wires the exchange. secret is the console's session key;
// codes are hashed under a key derived from it, so the database alone does
// not give them back (joincode.Key).
func NewJoinService(codes ports.JoinCodeStore, cfg *ConfigService, creds *DeviceCredentials,
	clock ports.Clock, tenant string, secret []byte) *JoinService {
	return &JoinService{codes: codes, cfg: cfg, creds: creds, clock: clock, tenant: tenant,
		key: joincode.Key(secret), rand: rand.Reader}
}

// JoinRefusal is a join the console understood and turned down. Reason is
// written for the person at the laptop, who cannot see the console.
type JoinRefusal struct{ Reason string }

func (e *JoinRefusal) Error() string { return e.Reason }

// ErrCodeRefused answers every code that cannot be used - mistyped, used up,
// withdrawn or expired - with the same words, so the endpoint never tells a
// guesser which codes exist.
var ErrCodeRefused = &JoinRefusal{Reason: "this code does not work: it is mistyped, used up or expired. Ask for a new one."}

func refuseJoin(format string, args ...any) error {
	return &JoinRefusal{Reason: fmt.Sprintf(format, args...)}
}

// NewCode is what "Add a laptop" asks. Group may be empty (the laptop joins
// at the organisation level); Tag makes a Re-enrol code for one device.
// Station makes a code for "Add a station" (design 0018): one use, and the
// machine that joins with it becomes an imaging station.
type NewCode struct {
	Group   string
	Uses    int
	Tag     string
	Station bool
	By      string
}

// Create makes a code and returns it as people read it (XXXX-XXXX). This is
// the only time it exists outside the laptop it is typed into.
func (s *JoinService) Create(ctx context.Context, req NewCode) (string, joincode.Code, error) {
	f := s.cfg.Fleet()
	if req.Station && req.Tag != "" {
		return "", joincode.Code{}, fmt.Errorf("a station code makes a new station; re-enrol %s from its own page", req.Tag)
	}
	if req.Station {
		req.Uses = 1
	}
	if req.Tag != "" {
		d, ok := f.Devices[req.Tag]
		if !ok {
			return "", joincode.Code{}, fmt.Errorf("unknown device %q", req.Tag)
		}
		if d.State == fleet.DeviceRetired {
			return "", joincode.Code{}, fmt.Errorf("%s is retired; reactivate it before it can join again", req.Tag)
		}
		req.Group, req.Uses = "", 1
	} else if req.Group != "" {
		if _, ok := f.Groups[req.Group]; !ok {
			return "", joincode.Code{}, fmt.Errorf("unknown group %q", req.Group)
		}
	}
	if req.Uses == 0 {
		req.Uses = 1
	}
	if req.Uses < 1 || req.Uses > joincode.MaxUses {
		return "", joincode.Code{}, fmt.Errorf("a code is for 1 to %d laptops", joincode.MaxUses)
	}
	code, err := joincode.New(s.rand)
	if err != nil {
		return "", joincode.Code{}, err
	}
	var id [8]byte
	if _, err := io.ReadFull(s.rand, id[:]); err != nil {
		return "", joincode.Code{}, fmt.Errorf("draw a code id: %w", err)
	}
	now := s.clock.Now()
	kind := ""
	if req.Station {
		kind = joincode.KindStation
	}
	c := joincode.Code{
		ID: hex.EncodeToString(id[:]), Hash: joincode.Hash(s.key, code),
		Group: req.Group, Tag: req.Tag, Kind: kind, Uses: req.Uses,
		Created: now, CreatedBy: req.By, Expires: now.Add(joincode.TTL),
	}
	if err := s.codes.Create(ctx, s.tenant, c); err != nil {
		return "", joincode.Code{}, err
	}
	return joincode.Format(code), c, nil
}

// Open lists the codes that can still be used.
func (s *JoinService) Open(ctx context.Context) ([]joincode.Code, error) {
	return s.codes.Open(ctx, s.tenant, s.clock.Now())
}

// Withdraw closes a code before its time.
func (s *JoinService) Withdraw(ctx context.Context, id string) error {
	return s.codes.Withdraw(ctx, s.tenant, id)
}

// JoinRequest is what a laptop says about itself. CPU is the vendor string
// the processor reports (GenuineIntel, AuthenticAMD) and Virtualisation what
// systemd-detect-virt says ("none" on a real machine); both only choose a
// generic hardware profile when no profile names the model.
type JoinRequest struct {
	Code           string
	Name           string
	Serial         string
	Vendor         string
	Model          string
	CPU            string
	Virtualisation string
}

// Joined is the answer: who the laptop is now, and how it proves it.
type Joined struct {
	Tag        string
	Credential string
}

// Join spends one use of the code and makes or finds the device. Any failure
// after the use was spent gives it back, so a laptop that hit a gate refusal
// can try again with the same code once the cause is fixed.
func (s *JoinService) Join(ctx context.Context, req JoinRequest) (out Joined, err error) {
	code, err := joincode.Normalize(req.Code)
	if err != nil {
		return Joined{}, ErrCodeRefused
	}
	now := s.clock.Now()
	c, ok, err := s.codes.Spend(ctx, s.tenant, joincode.Hash(s.key, code), now)
	if err != nil {
		return Joined{}, err
	}
	if !ok {
		return Joined{}, ErrCodeRefused
	}
	defer func() {
		if err == nil {
			return
		}
		if rerr := s.codes.Refund(context.WithoutCancel(ctx), s.tenant, c.ID); rerr != nil {
			err = errors.Join(err, rerr)
		}
	}()

	tag, fresh, err := s.place(c, req)
	if err != nil {
		return Joined{}, err
	}
	if fresh {
		hw, err := s.hardware(req)
		if err != nil {
			return Joined{}, err
		}
		serial, model := strings.TrimSpace(req.Serial), strings.TrimSpace(req.Model)
		d := fleet.Device{
			Hardware: hw,
			ITAM:     fleet.ITAM{Serial: serial, Model: model},
			Spec:     &fleet.HardwareSpec{Vendor: strings.TrimSpace(req.Vendor), Model: model, Serial: serial},
		}
		if c.Group != "" {
			d.Groups = []string{c.Group}
		}
		msg := fmt.Sprintf("devices: %s joins (%s) with a code from %s", tag, hw, c.CreatedBy)
		mut := fleet.AddDevice(tag, d, now)
		if c.Kind == joincode.KindStation {
			// The device and its station entry land in one commit, so there is
			// never a station without its machine or the other way round.
			d.Class = "station"
			msg = fmt.Sprintf("devices: %s joins (%s) as an imaging station with a code from %s", tag, hw, c.CreatedBy)
			mut = joinAsStation(tag, d, now)
		}
		by := ports.Author{Subject: "join:" + c.ID, Name: "sextant-join", Email: "join@sextant"}
		if err := s.cfg.Apply(ctx, mut, msg, by, tag); err != nil {
			return Joined{}, err
		}
	}
	secret, err := s.creds.Issue(ctx, tag)
	if err != nil {
		return Joined{}, fmt.Errorf("issue the credential for %s: %w", tag, err)
	}
	// A laptop that is no longer listening will never see this credential:
	// a proxy in front of the console gave up during a slow gate, or the
	// person pressed Ctrl-C. Treat that as a failed join so the use comes
	// back. The device, if it was committed, is now a provisional record with
	// this serial, so the laptop's next attempt is matched to it, skips the
	// gate and gets a credential of its own straight away.
	if err := ctx.Err(); err != nil {
		return Joined{}, fmt.Errorf("the laptop left before the answer: %w", err)
	}
	return Joined{Tag: tag, Credential: secret}, nil
}

// place decides which device the laptop becomes. fresh is true when a new
// record has to be made.
func (s *JoinService) place(c joincode.Code, req JoinRequest) (tag string, fresh bool, err error) {
	f := s.cfg.Fleet()
	serial := strings.TrimSpace(req.Serial)
	if c.Tag != "" {
		d, ok := f.Devices[c.Tag]
		if !ok || d.State == fleet.DeviceRetired {
			return "", false, refuseJoin("this code was made for %s, which is no longer in the fleet", c.Tag)
		}
		if joincode.Serial(serial) && joincode.Serial(d.ITAM.Serial) && !strings.EqualFold(serial, strings.TrimSpace(d.ITAM.Serial)) {
			return "", false, refuseJoin("this code is for %s (serial %s); this laptop has serial %s", c.Tag, d.ITAM.Serial, serial)
		}
		return c.Tag, false, nil
	}
	if joincode.Serial(serial) {
		if t, ok := f.ProvisionalBySerial(serial); ok {
			return t, false, nil
		}
		for _, t := range f.DeviceTags() {
			d := f.Devices[t]
			if d.State != fleet.DeviceRetired && strings.EqualFold(strings.TrimSpace(d.ITAM.Serial), serial) {
				return "", false, refuseJoin("this laptop is already enrolled as %s. To give it a new credential, use Re-enrol on its page in the console.", t)
			}
		}
	}
	prefix := "laptop-"
	if c.Kind == joincode.KindStation {
		prefix = "station-"
	}
	tag, err = s.newTag(f, req.Name, serial, prefix)
	return tag, true, err
}

// joinAsStation adds the device and registers it as an imaging station.
func joinAsStation(tag string, d fleet.Device, now time.Time) fleet.Mutation {
	return func(f *fleet.Fleet) error {
		if err := fleet.AddDevice(tag, d, now)(f); err != nil {
			return err
		}
		return fleet.AddStation(tag, fleet.Station{})(f)
	}
}

// newTag is the name the laptop asked for, or one made from its serial
// number, or a random one; a taken name made from the serial gets -2, -3.
func (s *JoinService) newTag(f *fleet.Fleet, name, serial, prefix string) (string, error) {
	if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
		if !fleet.ValidateSlug(name) {
			return "", refuseJoin("the name %q cannot be used: lower-case letters, digits and dashes, at most 63", name)
		}
		if taken(f, name) {
			return "", refuseJoin("the name %q is taken; choose another or leave it out", name)
		}
		return name, nil
	}
	base := joincode.TagFromSerial(serial)
	if base != "" {
		base = prefix + strings.TrimPrefix(base, "laptop-")
	}
	if base == "" {
		r, err := joincode.New(s.rand)
		if err != nil {
			return "", err
		}
		base = prefix + strings.ToLower(r[:6])
	}
	tag := base
	for i := 2; ; i++ {
		if !taken(f, tag) {
			return tag, nil
		}
		tag = fmt.Sprintf("%s-%d", base, i)
	}
}

// taken reports whether a tag names a device or a station already.
func taken(f *fleet.Fleet, tag string) bool {
	_, dev := f.Devices[tag]
	_, sta := f.Stations[tag]
	return dev || sta
}

// hardware picks the profile: one that names the model, otherwise the
// generic one for the processor, or vm for a virtual machine.
func (s *JoinService) hardware(req JoinRequest) (string, error) {
	hw := s.cfg.HardwareProfiles()
	if p := hw.Suggest(req.Vendor, req.Model); p != "" {
		return p, nil
	}
	generic := ""
	switch {
	case req.Virtualisation != "" && req.Virtualisation != "none":
		generic = "vm"
	case req.CPU == "GenuineIntel":
		generic = "generic-intel"
	case req.CPU == "AuthenticAMD":
		generic = "generic-amd"
	}
	if generic != "" && hw.Has(generic) {
		return generic, nil
	}
	return "", refuseJoin("no hardware profile fits this %s %s; add one to hardware-profiles.json in the fleet repository",
		strings.TrimSpace(req.Vendor), strings.TrimSpace(req.Model))
}

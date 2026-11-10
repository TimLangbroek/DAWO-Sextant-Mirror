package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/adapters/git"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/joincode"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/token"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// memJoinCodes is the store's contract in memory: spend only an open code,
// and one use at a time.
type memJoinCodes struct {
	mu sync.Mutex
	m  map[string]joincode.Code
}

func (s *memJoinCodes) Create(_ context.Context, _ string, c joincode.Code) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[c.ID] = c
	return nil
}

func (s *memJoinCodes) Spend(_ context.Context, _ string, hash []byte, now time.Time) (joincode.Code, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, c := range s.m {
		if bytes.Equal(c.Hash, hash) && c.Open(now) {
			before := c
			c.Uses--
			s.m[id] = c
			return before, true, nil
		}
	}
	return joincode.Code{}, false, nil
}

func (s *memJoinCodes) Refund(_ context.Context, _, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.m[id]; ok {
		c.Uses++
		s.m[id] = c
	}
	return nil
}

func (s *memJoinCodes) Open(_ context.Context, _ string, now time.Time) ([]joincode.Code, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []joincode.Code
	for _, c := range s.m {
		if c.Open(now) {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *memJoinCodes) Withdraw(_ context.Context, _, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.m[id]; ok {
		c.Uses = 0
		s.m[id] = c
	}
	return nil
}

const joinSeed = `{
  "version": 3,
  "groups": {"pilot": {}, "staff": {}},
  "devices": {
    "lt-active": {"groups": ["pilot"], "hardware": "generic-intel", "itam": {"serial": "SER-ACTIVE"}},
    "lt-prov":   {"groups": ["staff"], "hardware": "generic-intel", "state": "provisional", "itam": {"serial": "SER-PROV"}},
    "lt-old":    {"groups": ["pilot"], "hardware": "generic-intel", "state": "retired", "itam": {"serial": "SER-OLD"}},
    "laptop-taken": {"groups": ["pilot"], "hardware": "generic-intel"}
  }
}`

const joinProfiles = `[
  {"name": "thinkpad-t14", "vendor": "LENOVO", "models": ["21BR"]},
  {"name": "generic-intel"}, {"name": "vm"}
]`

type joinStack struct {
	svc   *JoinService
	cfg   *ConfigService
	creds *DeviceCredentials
	codes *memJoinCodes
	clock *fakeClock
	fail  *atomic.Bool // the gate refuses while set
	dir   string
}

func newJoinStack(t *testing.T) joinStack {
	t.Helper()
	dir := t.TempDir()
	sh(t, dir, "init", "-q", "-b", "main")
	for name, body := range map[string]string{"fleet.json": joinSeed, fleet.HardwareProfilesFile: joinProfiles} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sh(t, dir, "add", ".")
	sh(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "seed")
	repo, err := git.Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	fail := &atomic.Bool{}
	cfg, err := NewConfigService(repo, ports.GateFunc(func(context.Context, string, []string) error {
		if fail.Load() {
			return errors.New("the new host does not evaluate")
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	clock := newFakeClock(testT0)
	creds := NewDeviceCredentials(newMemTokenStore(), clock)
	codes := &memJoinCodes{m: map[string]joincode.Code{}}
	return joinStack{
		svc: NewJoinService(codes, cfg, creds, clock, "", []byte("session key")),
		cfg: cfg, creds: creds, codes: codes, clock: clock, fail: fail, dir: dir,
	}
}

func (st joinStack) code(t *testing.T, req NewCode) string {
	t.Helper()
	code, _, err := st.svc.Create(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

func (st joinStack) uses(t *testing.T) int {
	t.Helper()
	open, _ := st.codes.Open(context.Background(), "", st.clock.Now())
	n := 0
	for _, c := range open {
		n += c.Uses
	}
	return n
}

func refusal(err error) string {
	var r *JoinRefusal
	if errors.As(err, &r) {
		return r.Reason
	}
	return ""
}

func TestJoinMakesTheDeviceAndItsCredential(t *testing.T) {
	st := newJoinStack(t)
	ctx := context.Background()
	code := st.code(t, NewCode{Group: "pilot", By: "beheerder"})
	if len(code) != 9 || code[4] != '-' {
		t.Fatalf("code shown as %q, want XXXX-XXXX", code)
	}

	// Typed the way a person types it.
	typed := strings.ToLower(strings.ReplaceAll(code, "-", " "))
	got, err := st.svc.Join(ctx, JoinRequest{Code: typed, Serial: "PF3ABCDE", Vendor: "LENOVO", Model: "21BR ThinkPad T14", CPU: "GenuineIntel", Virtualisation: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Tag != "laptop-3abcde" {
		t.Fatalf("tag %q, want one made from the serial", got.Tag)
	}
	if tag, ok := st.creds.Authenticate(ctx, got.Credential); !ok || tag != got.Tag {
		t.Fatalf("the credential proves %q %v", tag, ok)
	}
	d := st.cfg.Fleet().Devices[got.Tag]
	if d.Hardware != "thinkpad-t14" || len(d.Groups) != 1 || d.Groups[0] != "pilot" ||
		d.ITAM.Serial != "PF3ABCDE" || d.ITAM.Model != "21BR ThinkPad T14" || !d.Provisional() ||
		d.Spec == nil || d.Spec.Vendor != "LENOVO" {
		t.Fatalf("device written as %+v", d)
	}
	if log := sh(t, st.dir, "log", "-1", "--format=%an %s"); log != "sextant-join devices: laptop-3abcde joins (thinkpad-t14) with a code from beheerder" {
		t.Fatalf("commit: %s", log)
	}

	// One laptop per code by default.
	if _, err := st.svc.Join(ctx, JoinRequest{Code: code, Serial: "OTHER1"}); !errors.Is(err, ErrCodeRefused) {
		t.Fatalf("a used code joined a second laptop: %v", err)
	}
}

func TestJoinRefusesMalformedExpiredAndWithdrawnCodesAlike(t *testing.T) {
	st := newJoinStack(t)
	ctx := context.Background()
	expired := st.code(t, NewCode{Group: "pilot"})
	st.clock.Advance(joincode.TTL)
	withdrawn, c, err := st.svc.Create(ctx, NewCode{Group: "pilot"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.svc.Withdraw(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	for name, code := range map[string]string{"expired": expired, "withdrawn": withdrawn, "malformed": "K7QM-4XW", "unknown": "0000-0000"} {
		if _, err := st.svc.Join(ctx, JoinRequest{Code: code, Serial: "NEW1"}); !errors.Is(err, ErrCodeRefused) {
			t.Errorf("%s code: %v, want the one refusal for every unusable code", name, err)
		}
	}
	if n := len(st.cfg.Fleet().Devices); n != 4 {
		t.Fatalf("refused codes made devices: %d", n)
	}
}

func TestJoinMatchesTheSerial(t *testing.T) {
	st := newJoinStack(t)
	ctx := context.Background()
	code := st.code(t, NewCode{Group: "pilot", Uses: 3})

	// A working laptop is never taken over by a code: the serial is not a secret.
	_, err := st.svc.Join(ctx, JoinRequest{Code: code, Serial: "ser-active ", CPU: "GenuineIntel"})
	if r := refusal(err); !strings.Contains(r, "already enrolled as lt-active") || !strings.Contains(r, "Re-enrol") {
		t.Fatalf("an active serial: %v", err)
	}
	if st.uses(t) != 3 {
		t.Fatalf("a refused join kept its use: %d left", st.uses(t))
	}

	// The same unfinished join is the same device.
	got, err := st.svc.Join(ctx, JoinRequest{Code: code, Serial: "SER-PROV", CPU: "GenuineIntel"})
	if err != nil || got.Tag != "lt-prov" {
		t.Fatalf("a provisional serial: %+v %v", got, err)
	}
	if g := st.cfg.Fleet().Devices["lt-prov"].Groups; len(g) != 1 || g[0] != "staff" {
		t.Fatalf("reusing a provisional record moved it: %v", g)
	}

	// A retired laptop that comes back is a new device; its old record stays.
	got, err = st.svc.Join(ctx, JoinRequest{Code: code, Serial: "SER-OLD", CPU: "GenuineIntel"})
	if err != nil || got.Tag != "laptop-serold" {
		t.Fatalf("a retired serial: %+v %v", got, err)
	}
	if st.cfg.Fleet().Devices["lt-old"].State != fleet.DeviceRetired {
		t.Fatal("the retired record changed")
	}
	if st.uses(t) != 1 {
		t.Fatalf("%d uses left, want 1", st.uses(t))
	}
}

func TestJoinGivesTheUseBackWhenTheGateRefuses(t *testing.T) {
	st := newJoinStack(t)
	ctx := context.Background()
	code := st.code(t, NewCode{Group: "pilot"})
	st.fail.Store(true)
	if _, err := st.svc.Join(ctx, JoinRequest{Code: code, Serial: "PF3ABCDE", CPU: "GenuineIntel"}); err == nil {
		t.Fatal("a gate refusal joined")
	}
	if _, ok := st.cfg.Fleet().Devices["laptop-3abcde"]; ok {
		t.Fatal("a refused device is in the fleet")
	}
	if st.uses(t) != 1 {
		t.Fatal("the gate refusal spent the code")
	}
	// Fixed: the same code works.
	st.fail.Store(false)
	if got, err := st.svc.Join(ctx, JoinRequest{Code: code, Serial: "PF3ABCDE", CPU: "GenuineIntel"}); err != nil || got.Tag != "laptop-3abcde" {
		t.Fatalf("the same code after the fix: %+v %v", got, err)
	}
}

func TestReenrolCodeIsForOneDevice(t *testing.T) {
	st := newJoinStack(t)
	ctx := context.Background()
	old, err := st.creds.Issue(ctx, "lt-active")
	if err != nil {
		t.Fatal(err)
	}
	code := st.code(t, NewCode{Tag: "lt-active", Group: "staff", Uses: 5})

	// Another machine cannot use it.
	if r := refusal(firstErr(st.svc.Join(ctx, JoinRequest{Code: code, Serial: "SER-OTHER"}))); !strings.Contains(r, "this code is for lt-active") {
		t.Fatalf("a re-enrol code on another serial: %q", r)
	}
	got, err := st.svc.Join(ctx, JoinRequest{Code: code, Serial: "SER-ACTIVE"})
	if err != nil || got.Tag != "lt-active" {
		t.Fatalf("re-enrol: %+v %v", got, err)
	}
	if _, ok := st.creds.Authenticate(ctx, old); ok {
		t.Fatal("the old credential still works after re-enrol")
	}
	if _, ok := st.creds.Authenticate(ctx, got.Credential); !ok {
		t.Fatal("the new credential does not work")
	}
	// A re-enrol code is for one use, whatever was asked.
	if _, err := st.svc.Join(ctx, JoinRequest{Code: code, Serial: "SER-ACTIVE"}); !errors.Is(err, ErrCodeRefused) {
		t.Fatalf("a re-enrol code worked twice: %v", err)
	}
	if g := st.cfg.Fleet().Devices["lt-active"].Groups; len(g) != 1 || g[0] != "pilot" {
		t.Fatalf("re-enrol moved the device: %v", g)
	}

	// A device retired after its code was made cannot use it.
	code = st.code(t, NewCode{Tag: "lt-active"})
	if err := st.cfg.ApplyStructural(ctx, fleet.RetireDevice("lt-active"), "retire", ports.Author{Name: "t"}); err != nil {
		t.Fatal(err)
	}
	if r := refusal(firstErr(st.svc.Join(ctx, JoinRequest{Code: code, Serial: "SER-ACTIVE"}))); !strings.Contains(r, "no longer in the fleet") {
		t.Fatalf("a re-enrol code for a device retired since: %q", r)
	}
}

func firstErr(_ Joined, err error) error { return err }

func TestJoinNamesAndProfiles(t *testing.T) {
	st := newJoinStack(t)
	ctx := context.Background()
	code := st.code(t, NewCode{Group: "pilot", Uses: 10})
	join := func(req JoinRequest) (Joined, error) {
		req.Code = code
		return st.svc.Join(ctx, req)
	}
	for _, tc := range []struct {
		name    string
		req     JoinRequest
		tag, hw string
		refused string
	}{
		{"asked name", JoinRequest{Name: " Bram-Laptop ", Serial: "S1", CPU: "GenuineIntel"}, "bram-laptop", "generic-intel", ""},
		{"name taken", JoinRequest{Name: "lt-active", Serial: "S2", CPU: "GenuineIntel"}, "", "", `the name "lt-active" is taken`},
		{"name unusable", JoinRequest{Name: "bram's laptop", Serial: "S3", CPU: "GenuineIntel"}, "", "", "cannot be used"},
		{"serial tag taken", JoinRequest{Serial: "TAKEN", CPU: "GenuineIntel", Virtualisation: "none"}, "laptop-taken-2", "generic-intel", ""},
		{"no generic profile", JoinRequest{Serial: "S5", Vendor: "Acme", Model: "A1", CPU: "AuthenticAMD"}, "", "", "no hardware profile fits this Acme A1"},
		{"virtual", JoinRequest{Serial: "VM1", CPU: "GenuineIntel", Virtualisation: "kvm"}, "laptop-vm1", "vm", ""},
		{"no profile", JoinRequest{Serial: "S4", Vendor: "Acme", Model: "Rocket 9", CPU: "VIA"}, "", "", "no hardware profile fits this Acme Rocket 9"},
	} {
		got, err := join(tc.req)
		if tc.refused != "" {
			if r := refusal(err); !strings.Contains(r, tc.refused) {
				t.Errorf("%s: %v, want a refusal with %q", tc.name, err, tc.refused)
			}
			continue
		}
		if err != nil || got.Tag != tc.tag {
			t.Errorf("%s: %+v %v, want %s", tc.name, got, err, tc.tag)
			continue
		}
		if hw := st.cfg.Fleet().Devices[got.Tag].Hardware; hw != tc.hw {
			t.Errorf("%s: profile %s, want %s", tc.name, hw, tc.hw)
		}
	}

	// No usable serial: a random name, still a slug.
	for _, serial := range []string{"", "Default string"} {
		got, err := join(JoinRequest{Serial: serial, CPU: "GenuineIntel"})
		if err != nil || !strings.HasPrefix(got.Tag, "laptop-") || len(got.Tag) != len("laptop-")+6 || !fleet.ValidateSlug(got.Tag) {
			t.Errorf("serial %q: %+v %v", serial, got, err)
		}
	}
}

func TestCreateChecksWhatTheCodeIsFor(t *testing.T) {
	st := newJoinStack(t)
	ctx := context.Background()
	for name, req := range map[string]NewCode{
		"unknown group":  {Group: "nope"},
		"too many":       {Group: "pilot", Uses: joincode.MaxUses + 1},
		"negative":       {Group: "pilot", Uses: -1},
		"unknown device": {Tag: "nope"},
		"retired device": {Tag: "lt-old"},
	} {
		if _, _, err := st.svc.Create(ctx, req); err == nil {
			t.Errorf("%s: a code was made", name)
		}
	}
	_, c, err := st.svc.Create(ctx, NewCode{Group: "pilot", By: "beheerder"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Uses != 1 || !c.Expires.Equal(testT0.Add(joincode.TTL)) || c.CreatedBy != "beheerder" || len(c.ID) != 16 {
		t.Fatalf("code stored as %+v", c)
	}
	if c.Hash == nil || bytes.Contains(c.Hash, []byte(c.ID)) {
		t.Fatal("no hash stored")
	}
}

// cancelOnPut ends the request at the moment the credential is stored: the
// device exists and has a credential, and the laptop is gone.
type cancelOnPut struct {
	ports.TokenStore
	cancel func()
}

func (c cancelOnPut) Put(ctx context.Context, tok token.Token) error {
	c.cancel()
	return c.TokenStore.Put(ctx, tok)
}

func TestJoinALaptopThatLeftTriesAgain(t *testing.T) {
	st := newJoinStack(t)
	code := st.code(t, NewCode{Group: "pilot"})
	tokens := newMemTokenStore()
	ctx, cancel := context.WithCancel(context.Background())
	st.svc.creds = NewDeviceCredentials(cancelOnPut{tokens, cancel}, st.clock)
	if _, err := st.svc.Join(ctx, JoinRequest{Code: code, Serial: "PF3ABCDE", CPU: "GenuineIntel"}); err == nil {
		t.Fatal("a join whose laptop left reported success")
	}
	if st.uses(t) != 1 {
		t.Fatal("the use went to a laptop that never got its answer")
	}
	if !st.cfg.Fleet().Devices["laptop-3abcde"].Provisional() {
		t.Fatal("the device was not committed before the laptop left; this test no longer tests the late case")
	}

	// The next attempt is matched on the serial and gets its own credential.
	creds := NewDeviceCredentials(tokens, st.clock)
	st.svc.creds = creds
	got, err := st.svc.Join(context.Background(), JoinRequest{Code: code, Serial: "PF3ABCDE", CPU: "GenuineIntel"})
	if err != nil || got.Tag != "laptop-3abcde" {
		t.Fatalf("the second attempt: %+v %v", got, err)
	}
	if tag, ok := creds.Authenticate(context.Background(), got.Credential); !ok || tag != got.Tag {
		t.Fatal("the second attempt's credential does not work")
	}
}

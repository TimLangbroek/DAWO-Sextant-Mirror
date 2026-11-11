package main

// install.go: what the installation USB does at the laptop (design 0016),
// in the order the person sees it: network, code, confirm, install, hand
// over. Every effect goes through the installer's seams (run, files, the
// terminal), so the order and the refusals are tested without a disk.

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The contract with the Nix side (sextant.lib.mkInstaller and the device
// module's default disk layout). Change one, change both.
const (
	// diskKeyFile is the LUKS passphrase file the default layout's disko
	// configuration reads while it formats.
	diskKeyFile = "/tmp/sextant-disk.key"
	// defaultDiskName is the disk the default layout calls its one disk;
	// the installer points it at the disk it chose.
	defaultDiskName = "main"
	// joinedDir is where join writes before there is a disk to write to.
	joinedDir = "/tmp/joined"
	// fleetClone is where the fleet repository is cloned to install from.
	fleetClone = "/tmp/fleet"
	// handoverDir holds what goes into the installed system's agent state.
	handoverDir = "/tmp/handover"
	// agentStateDir is that state directory on the installed system.
	agentStateDir = "/var/lib/sextant-agent"
	// enrolKeyPath is where sextant-actd finds the key that lets it seal
	// the disk to PCR 7 when the laptop is secured.
	enrolKeyPath = "/var/lib/sextant-actd/luks-enroll.key"
	// sbctlDir holds the per-device Secure Boot keys: sbctl creates them in
	// the live system, lanzaboote signs the installed boot chain with them
	// and sextant-actd enrols them when the firmware is in Setup Mode.
	sbctlDir = "/var/lib/sbctl"
)

// runResult is what one command did.
type runResult struct {
	Out  string // stdout and stderr together
	Code int    // exit code; -1 when it did not start
}

// installer holds the seams.
type installer struct {
	console string
	in      *bufio.Reader
	out     io.Writer
	// run starts a command with extra environment and waits for it.
	run func(ctx context.Context, env []string, name string, args ...string) runResult
	// reachable reports whether the console answers.
	reachable func(ctx context.Context) bool
	// hasTPM2 reports whether the laptop has a TPM2.
	hasTPM2 func() bool
	// root is prefixed to every path the installer writes; "" in production.
	root string
	// station, when set, is the imaging station this machine booted from
	// (design 0018): the job comes from there instead of a typed code.
	station *stationClient
}

// joined is what join left in joined.json.
type joined struct {
	Tag    string `json:"tag"`
	URL    string `json:"url"`
	Branch string `json:"branch"`
}

// disk is a candidate to install on.
type disk struct {
	Path  string
	Model string
	Bytes int64
}

func (d disk) String() string {
	model := d.Model
	if model == "" {
		model = d.Path
	}
	return fmt.Sprintf("%s (%d GB)", model, d.Bytes/1_000_000_000)
}

// Run is the whole stick, start to reboot. It returns only when something
// cannot go on; a refused code or a missing network is asked again.
func (i *installer) Run(ctx context.Context) error {
	if i.station != nil {
		return i.runFromStation(ctx)
	}
	i.say("Sextant installation\n")
	if err := i.network(ctx); err != nil {
		return err
	}
	j, err := i.join(ctx)
	if err != nil {
		return err
	}
	d, err := i.pickDisk(ctx)
	if err != nil {
		return err
	}
	i.say("\nThis laptop becomes %s.\nEverything on %s is erased.\n", j.Tag, d)
	if !i.yes("Go on? [y/N] ") {
		return errors.New("stopped before anything was erased")
	}
	if err := i.install(ctx, j, d, ""); err != nil {
		return err
	}
	i.say("\nInstalled. The laptop restarts and appears in the console as %s.\n", j.Tag)
	i.run(ctx, nil, "systemctl", "reboot")
	return nil
}

// network waits until the console answers, offering wireless networks while
// it does not. Wired needs nothing from anybody.
func (i *installer) network(ctx context.Context) error {
	for !i.reachable(ctx) {
		i.say("\nNo connection to %s.\n", i.console)
		r := i.run(ctx, nil, "nmcli", "-t", "-f", "SSID", "device", "wifi", "list", "--rescan", "yes")
		var nets []string
		seen := map[string]bool{}
		for _, l := range strings.Split(r.Out, "\n") {
			if l = strings.TrimSpace(l); l != "" && !seen[l] {
				seen[l] = true
				nets = append(nets, l)
			}
		}
		if len(nets) == 0 {
			if !i.yes("No wireless networks found. Plug in a cable and press y to try again: ") {
				return errors.New("no network")
			}
			continue
		}
		for n, name := range nets {
			i.say("  %d  %s\n", n+1, name)
		}
		n, err := strconv.Atoi(i.ask("Network number: "))
		if err != nil || n < 1 || n > len(nets) {
			continue
		}
		pw := i.ask("Password: ")
		if r := i.run(ctx, nil, "nmcli", "device", "wifi", "connect", nets[n-1], "password", pw); r.Code != 0 {
			i.say("Could not connect: %s\n", strings.TrimSpace(r.Out))
		}
	}
	return nil
}

// join asks for the code until the console takes one. Nothing has been
// written to the disk yet, so a wrong code costs nothing.
func (i *installer) join(ctx context.Context) (joined, error) {
	for {
		code := i.ask("\nType the code from the console: ")
		if code == "" {
			continue
		}
		r := i.run(ctx, nil, "sextant-agent", "join", i.console, code, "--state-dir", i.path(joinedDir))
		switch r.Code {
		case 0:
			var j joined
			raw, err := os.ReadFile(i.path(filepath.Join(joinedDir, "joined.json")))
			if err == nil {
				err = json.Unmarshal(raw, &j)
			}
			if err != nil {
				return joined{}, fmt.Errorf("join answered but left no laptop name: %w", err)
			}
			if j.Tag == "" {
				return joined{}, errors.New("join answered but left no laptop name")
			}
			return j, nil
		case 4:
			// Refused: the console's own words, then ask again.
			i.say("%s\n", strings.TrimSpace(r.Out))
		case 5:
			i.say("The console could not be reached. Check the network and type the code again.\n")
		default:
			return joined{}, fmt.Errorf("join failed (exit %d): %s", r.Code, strings.TrimSpace(r.Out))
		}
	}
}

// pickDisk takes the largest internal disk that is not the stick itself.
func (i *installer) pickDisk(ctx context.Context) (disk, error) {
	r := i.run(ctx, nil, "lsblk", "-J", "-b", "-d", "-p", "-o", "NAME,SIZE,MODEL,RM,TYPE,TRAN")
	if r.Code != 0 {
		return disk{}, fmt.Errorf("cannot list disks: %s", strings.TrimSpace(r.Out))
	}
	var l struct {
		Blockdevices []struct {
			Name  string `json:"name"`
			Size  int64  `json:"size"`
			Model string `json:"model"`
			RM    bool   `json:"rm"`
			Type  string `json:"type"`
			Tran  string `json:"tran"`
		} `json:"blockdevices"`
	}
	if err := json.Unmarshal([]byte(r.Out), &l); err != nil {
		return disk{}, fmt.Errorf("cannot read the disk list: %w", err)
	}
	stick := i.stickDisk(ctx)
	ds := make([]disk, 0, len(l.Blockdevices))
	for _, b := range l.Blockdevices {
		if b.Type != "disk" || b.RM || b.Tran == "usb" || b.Name == stick {
			continue
		}
		ds = append(ds, disk{Path: b.Name, Model: strings.TrimSpace(b.Model), Bytes: b.Size})
	}
	if len(ds) == 0 {
		return disk{}, errors.New("this laptop has no internal disk to install on")
	}
	sort.SliceStable(ds, func(a, b int) bool { return ds[a].Bytes > ds[b].Bytes })
	return ds[0], nil
}

// stickDisk is the disk the live system booted from, so it is never chosen
// even when it does not say it is removable.
func (i *installer) stickDisk(ctx context.Context) string {
	src := strings.TrimSpace(i.run(ctx, nil, "findmnt", "-n", "-o", "SOURCE", "/iso").Out)
	if src == "" {
		return ""
	}
	parent := strings.TrimSpace(i.run(ctx, nil, "lsblk", "-n", "-p", "-o", "PKNAME", src).Out)
	if parent == "" {
		return src
	}
	return parent
}

// install clones the fleet, lays out and installs the disk the fleet says,
// and binds it to the TPM2 when there is one. rev pins the fleet revision a
// station job names; "" takes the branch head.
func (i *installer) install(ctx context.Context, j joined, d disk, rev string) error {
	cred, err := os.ReadFile(i.path(filepath.Join(joinedDir, "credential")))
	if err != nil {
		return fmt.Errorf("join left no credential: %w", err)
	}
	phrase, err := newPhrase()
	if err != nil {
		return err
	}

	// What the installed laptop starts with: its credential, its name, the
	// phrase for the console to escrow, and the key actd seals PCR 7 with.
	hand := i.path(handoverDir)
	state := filepath.Join(hand, agentStateDir)
	for name, body := range map[string][]byte{
		filepath.Join(state, "credential"):   cred,
		filepath.Join(state, "joined.json"):  mustRead(i.path(filepath.Join(joinedDir, "joined.json"))),
		filepath.Join(state, "recovery.key"): []byte(phrase),
		filepath.Join(hand, enrolKeyPath):    []byte(phrase),
		i.path(diskKeyFile):                  []byte(phrase),
	} {
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(name, body, 0o600); err != nil {
			return err
		}
	}

	// Per-device Secure Boot keys, made here so lanzaboote signs the boot
	// chain at install and actd can enrol them later: at first boot for a
	// machine from a station whose firmware is in Setup Mode, or when an
	// owner secures the laptop.
	if r := i.run(ctx, nil, "sbctl", "create-keys"); r.Code != 0 {
		return fmt.Errorf("could not make the Secure Boot keys: %s", tail(r.Out))
	}
	if err := os.MkdirAll(filepath.Join(hand, filepath.Dir(sbctlDir)), 0o700); err != nil {
		return err
	}
	if r := i.run(ctx, nil, "cp", "-a", i.path(sbctlDir), filepath.Join(hand, sbctlDir)); r.Code != 0 {
		return fmt.Errorf("could not stage the Secure Boot keys: %s", tail(r.Out))
	}

	i.step(ctx, "Fetching the fleet configuration", 10)
	i.say("\nFetching the fleet configuration...\n")
	auth := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(j.Tag+":"+string(cred)))
	// The credential goes in through the environment, never the argument
	// list, which any process on the machine can read.
	env := []string{"GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.extraHeader", "GIT_CONFIG_VALUE_0=" + auth}
	branch := j.Branch
	if branch == "" {
		branch = "main"
	}
	repo := strings.TrimSuffix(j.URL, "/") + "/fleet.git"
	if rev == "" {
		if r := i.run(ctx, env, "git", "clone", "--depth", "1", "--branch", branch, repo, i.path(fleetClone)); r.Code != 0 {
			return fmt.Errorf("could not fetch the fleet configuration: %s", strings.TrimSpace(r.Out))
		}
	} else {
		// The revision the console approved, not whatever the branch says
		// now: a device installed ahead of its ring cannot converge back.
		if r := i.run(ctx, env, "git", "clone", "--no-checkout", repo, i.path(fleetClone)); r.Code != 0 {
			return fmt.Errorf("could not fetch the fleet configuration: %s", strings.TrimSpace(r.Out))
		}
		if r := i.run(ctx, nil, "git", "-C", i.path(fleetClone), "checkout", "--detach", rev); r.Code != 0 {
			return fmt.Errorf("the fleet configuration has no revision %s: %s", rev, strings.TrimSpace(r.Out))
		}
	}

	i.step(ctx, "Installing the system", 30)
	i.say("Installing on %s. This takes a while.\n", d)
	if r := i.run(ctx, nil, "disko-install", "--flake", i.path(fleetClone)+"#"+j.Tag,
		"--disk", defaultDiskName, d.Path, "--extra-files", hand, "/",
		"--write-efi-boot-entries"); r.Code != 0 {
		return fmt.Errorf("installation failed: %s", tail(r.Out))
	}

	if !i.hasTPM2() {
		i.say("This laptop has no TPM2: it asks for its disk phrase at every start.\nThe console holds the phrase; an owner can reveal it.\n")
		return nil
	}
	luks := strings.Fields(i.run(ctx, nil, "blkid", "-t", "TYPE=crypto_LUKS", "-o", "device").Out)
	var mine []string
	for _, p := range luks {
		if strings.HasPrefix(p, d.Path) {
			mine = append(mine, p)
		}
	}
	if len(mine) != 1 {
		i.say("Found %d encrypted volumes on %s, so the TPM2 is not bound; the laptop asks for its phrase until it is secured.\n", len(mine), d.Path)
		return nil
	}
	i.step(ctx, "Binding the disk to the TPM2", 90)
	// Bound to no PCR on purpose (design 0016, decided): the laptop starts
	// by itself until Secure this laptop seals to PCR 7 and removes this slot.
	if r := i.run(ctx, nil, "systemd-cryptenroll", "--unlock-key-file="+i.path(diskKeyFile),
		"--tpm2-device=auto", "--tpm2-pcrs=", mine[0]); r.Code != 0 {
		i.say("The TPM2 could not be bound (%s); the laptop asks for its phrase until it is secured.\n", tail(r.Out))
	}
	return nil
}

// phraseAlphabet: thirty-two characters nobody mistakes for one another (no
// I, O, 0 or 1), as for the administrator password.
const phraseAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// newPhrase draws five groups of five: 125 bits, typeable at every start on a
// laptop without a TPM2.
func newPhrase() (string, error) {
	b := make([]byte, 25)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("no randomness for the disk phrase: %w", err)
	}
	var s strings.Builder
	for n, c := range b {
		if n > 0 && n%5 == 0 {
			s.WriteByte('-')
		}
		s.WriteByte(phraseAlphabet[int(c)%len(phraseAlphabet)])
	}
	return s.String(), nil
}

func (i *installer) path(p string) string { return filepath.Join(i.root, p) }

func (i *installer) say(format string, a ...any) { _, _ = fmt.Fprintf(i.out, format, a...) }

func (i *installer) ask(prompt string) string {
	i.say("%s", prompt)
	line, _ := i.in.ReadString('\n')
	return strings.TrimSpace(line)
}

func (i *installer) yes(prompt string) bool {
	switch strings.ToLower(i.ask(prompt)) {
	case "y", "yes", "j", "ja":
		return true
	}
	return false
}

func mustRead(p string) []byte {
	b, _ := os.ReadFile(p)
	return b
}

// tail keeps the last lines of a long tool output: the sentence that says
// what went wrong is at the end.
func tail(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 5 {
		lines = lines[len(lines)-5:]
	}
	return strings.Join(lines, "\n")
}

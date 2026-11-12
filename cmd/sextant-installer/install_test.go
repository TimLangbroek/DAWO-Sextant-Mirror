package main

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const lsblkThree = `{"blockdevices":[
 {"name":"/dev/sda","size":32000000000,"model":"USB stick","rm":true,"type":"disk","tran":"usb"},
 {"name":"/dev/nvme0n1","size":512110190592,"model":"Samsung PM9A1","rm":false,"type":"disk","tran":"nvme"},
 {"name":"/dev/sdb","size":256060514304,"model":"Small SSD","rm":false,"type":"disk","tran":"sata"},
 {"name":"/dev/loop0","size":1000000000,"model":null,"rm":false,"type":"loop","tran":null}]}`

// rig is an installer whose commands are answered by the test.
type rig struct {
	*installer
	calls []string
	envs  map[string][]string
	out   strings.Builder
	// joinAnswers are the exit codes join gives, in turn.
	joinAnswers []int
	tpm         bool
	online      []bool
	answer      map[string]runResult
	// escrowFails makes the console not confirm the phrase on --once.
	escrowFails bool
	escrowed    string // the phrase the --once check-in found
}

func newRig(t *testing.T, input string) *rig {
	t.Helper()
	r := &rig{envs: map[string][]string{}, tpm: true, answer: map[string]runResult{
		"lsblk -J":  {Out: lsblkThree},
		"findmnt":   {Out: "/dev/sda1\n"},
		"lsblk -n":  {Out: "/dev/sda\n"},
		"blkid":     {Out: "/dev/nvme0n1p2\n/dev/sdb1\n"},
		"nmcli -t":  {Out: "home\nhome\noffice\n"},
		"git":       {},
		"sbctl":     {},
		"cp -a":     {},
		"disko":     {},
		"systemd":   {},
		"systemctl": {},
	}}
	root := t.TempDir()
	r.installer = &installer{
		console: "https://console.example.test",
		in:      bufio.NewReader(strings.NewReader(input)),
		out:     &r.out,
		root:    root,
		hasTPM2: func() bool { return r.tpm },
		reachable: func(context.Context) bool {
			if len(r.online) == 0 {
				return true
			}
			up := r.online[0]
			r.online = r.online[1:]
			return up
		},
	}
	r.run = func(_ context.Context, env []string, name string, args ...string) runResult {
		call := strings.Join(append([]string{name}, args...), " ")
		r.calls = append(r.calls, call)
		r.envs[name] = env
		if name == "sextant-agent" && len(args) > 0 && args[0] == "--once" {
			rec := filepath.Join(root, recoveryFile)
			b, _ := os.ReadFile(rec)
			r.escrowed = string(b)
			if r.escrowFails {
				return runResult{Out: "sextant-agent: console unreachable", Code: 1}
			}
			_ = os.Remove(rec) // what the agent does on X-Recovery-Key-Stored
			return runResult{}
		}
		if name == "sextant-agent" {
			code := 0
			if len(r.joinAnswers) > 0 {
				code, r.joinAnswers = r.joinAnswers[0], r.joinAnswers[1:]
			}
			if code == 4 {
				return runResult{Out: "this code does not work: it is mistyped, used up or expired\n", Code: 4}
			}
			dir := filepath.Join(root, joinedDir)
			_ = os.MkdirAll(dir, 0o700)
			_ = os.WriteFile(filepath.Join(dir, "credential"), []byte("cred-xyz"), 0o600)
			_ = os.WriteFile(filepath.Join(dir, "joined.json"),
				[]byte(`{"tag":"laptop-3abcde","url":"https://console.example.test","branch":"main"}`), 0o644)
			return runResult{}
		}
		for prefix, res := range r.answer {
			if strings.HasPrefix(call, prefix) {
				return res
			}
		}
		return runResult{Out: "unexpected " + call, Code: 127}
	}
	return r
}

func (r *rig) called(prefix string) string {
	for _, c := range r.calls {
		if strings.HasPrefix(c, prefix) {
			return c
		}
	}
	return ""
}

func (r *rig) file(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.root, p))
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	return string(b)
}

// A wrong code is asked again; the right one installs the largest internal
// disk, never the stick, with the fleet's layout, and the laptop gets its
// credential, its name and the phrase the console escrows.
func TestTheStickInstallsTheLaptop(t *testing.T) {
	r := newRig(t, "WRONG-CODE\nK7QM-4XWD\ny\n")
	r.joinAnswers = []int{4, 0}
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, r.out.String())
	}
	out := r.out.String()
	if !strings.Contains(out, "this code does not work") {
		t.Error("a refused code was not shown as the console worded it")
	}
	if !strings.Contains(out, "This laptop becomes laptop-3abcde.") || !strings.Contains(out, "Everything on Samsung PM9A1 (512 GB) is erased.") {
		t.Errorf("confirmation:\n%s", out)
	}

	clone := r.called("git clone")
	if !strings.Contains(clone, "https://console.example.test/fleet.git") || strings.Contains(clone, "cred-xyz") {
		t.Errorf("clone %q (the credential must not be in the arguments)", clone)
	}
	if env := strings.Join(r.envs["git"], " "); !strings.Contains(env, "GIT_CONFIG_VALUE_0=Authorization: Basic ") {
		t.Errorf("clone environment %q carries no credential", env)
	}
	disko := r.called("disko-install")
	for _, want := range []string{"#laptop-3abcde", "--disk main /dev/nvme0n1", "--extra-files " + filepath.Join(r.root, handoverDir) + "/. /"} {
		if !strings.Contains(disko, want) {
			t.Errorf("disko-install %q lacks %q", disko, want)
		}
	}

	// The Secure Boot keys are made and staged before the install, so
	// lanzaboote signs the boot chain with them.
	if cp := r.called("cp -a"); cp != "cp -a "+filepath.Join(r.root, sbctlDir)+" "+filepath.Join(r.root, handoverDir, sbctlDir) {
		t.Errorf("keys staged with %q", cp)
	}
	if order := strings.Join(r.calls, "|"); strings.Index(order, "sbctl create-keys") > strings.Index(order, "disko-install") {
		t.Error("the Secure Boot keys were made after the install")
	}

	phrase := r.file(t, diskKeyFile)
	if !regexp.MustCompile(`^[A-HJ-NP-Z2-9]{5}(-[A-HJ-NP-Z2-9]{5}){4}$`).MatchString(phrase) {
		t.Errorf("phrase %q", phrase)
	}
	state := filepath.Join(handoverDir, agentStateDir)
	if got := r.file(t, filepath.Join(state, "credential")); got != "cred-xyz" {
		t.Errorf("credential handed over %q", got)
	}
	if !strings.Contains(r.file(t, filepath.Join(state, "joined.json")), "laptop-3abcde") {
		t.Error("joined.json not handed over")
	}
	if r.file(t, filepath.Join(state, "recovery.key")) != phrase || r.file(t, filepath.Join(handoverDir, enrolKeyPath)) != phrase {
		t.Error("the escrowed phrase or the enrol key is not the disk's phrase")
	}

	enrol := r.called("systemd-cryptenroll")
	if !strings.HasSuffix(enrol, "--tpm2-device=auto --tpm2-pcrs= /dev/nvme0n1p2") {
		t.Errorf("TPM2 enrol %q, want the new disk's volume bound to no PCR", enrol)
	}
	if r.escrowed != phrase {
		t.Errorf("the check-in before the restart escrowed %q, want the disk's phrase", r.escrowed)
	}
	if env := strings.Join(r.envs["sextant-agent"], " "); !strings.Contains(env, "SEXTANT_TAG=laptop-3abcde") ||
		!strings.Contains(env, "SEXTANT_CREDENTIAL_FILE="+filepath.Join(r.root, joinedDir, "credential")) {
		t.Errorf("escrow check-in environment %q", env)
	}
	if order := strings.Join(r.calls, "|"); strings.Index(order, "sextant-agent --once") < strings.Index(order, "disko-install") {
		t.Error("the phrase was escrowed before the disk it opens existed")
	}
	if strings.Contains(r.out.String(), phrase) {
		t.Error("the phrase was shown although the console holds it")
	}
	if r.calls[len(r.calls)-1] != "systemctl reboot" {
		t.Errorf("last call %q, want the restart", r.calls[len(r.calls)-1])
	}
}

// When the console does not confirm the phrase, the person at the laptop
// writes it down before it restarts: a laptop without a TPM2 asks for it.
func TestAnUnconfirmedPhraseIsShownBeforeTheRestart(t *testing.T) {
	r := newRig(t, "K7QM-4XWD\ny\n\n")
	r.tpm = false
	r.escrowFails = true
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, r.out.String())
	}
	phrase := r.escrowed
	out := r.out.String()
	if phrase == "" || !strings.Contains(out, "Write it down now") || !strings.Contains(out, "    "+phrase+"\n") {
		t.Fatalf("the phrase was not shown:\n%s", out)
	}
	if strings.Contains(out, "The console holds the disk phrase") {
		t.Error("claimed the console holds a phrase it did not confirm")
	}
	if _, err := os.Stat(filepath.Join(r.root, recoveryFile)); !os.IsNotExist(err) {
		t.Error("the phrase stays in the live system's agent directory")
	}
}

// Anything but yes stops before the disk is touched.
func TestNoMeansNothingIsErased(t *testing.T) {
	r := newRig(t, "K7QM-4XWD\n\n")
	if err := r.Run(context.Background()); err == nil {
		t.Fatal("an empty answer went on")
	}
	if r.called("git") != "" || r.called("disko-install") != "" {
		t.Errorf("calls after no: %v", r.calls)
	}
}

// Without a TPM2 the laptop is installed and asks for its phrase.
func TestALaptopWithoutTPM2AsksForThePhrase(t *testing.T) {
	r := newRig(t, "K7QM-4XWD\ny\n")
	r.tpm = false
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.called("systemd-cryptenroll") != "" {
		t.Error("enrolled a TPM2 that is not there")
	}
	if !strings.Contains(r.out.String(), "asks for its disk phrase at every start") {
		t.Error("the person is not told about the phrase")
	}
}

// Without a network the stick offers the wireless networks, each once.
func TestNoNetworkOffersWireless(t *testing.T) {
	r := newRig(t, "2\nsecret\nK7QM-4XWD\ny\n")
	r.online = []bool{false, true}
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.Count(r.out.String(), "home") != 1 {
		t.Errorf("networks listed:\n%s", r.out.String())
	}
	if c := r.called("nmcli device wifi connect"); c != "nmcli device wifi connect office password secret" {
		t.Errorf("connect %q", c)
	}
}

// The stick is never chosen, even when it does not call itself removable,
// and a laptop with no other disk is refused.
func TestTheStickIsNeverTheTarget(t *testing.T) {
	r := newRig(t, "")
	r.answer["lsblk -J"] = runResult{Out: `{"blockdevices":[
 {"name":"/dev/sdc","size":900000000000,"model":"Big stick","rm":false,"type":"disk","tran":"sata"},
 {"name":"/dev/sdb","size":256060514304,"model":"Small SSD","rm":false,"type":"disk","tran":"sata"}]}`}
	r.answer["findmnt"] = runResult{Out: "/dev/sdc1\n"}
	r.answer["lsblk -n"] = runResult{Out: "/dev/sdc\n"}
	d, err := r.pickDisk(context.Background())
	if err != nil || d.Path != "/dev/sdb" {
		t.Fatalf("picked %+v, %v", d, err)
	}
	r.answer["lsblk -J"] = runResult{Out: `{"blockdevices":[{"name":"/dev/sdc","size":9,"rm":false,"type":"disk","tran":"sata"}]}`}
	if _, err := r.pickDisk(context.Background()); err == nil {
		t.Error("installed on the stick")
	}
}

func TestPhraseIsUniformOverTheAlphabet(t *testing.T) {
	if 256%len(phraseAlphabet) != 0 {
		t.Fatal("the alphabet does not divide a byte: the phrase would be biased")
	}
	seen := map[string]bool{}
	for n := 0; n < 50; n++ {
		p, err := newPhrase()
		if err != nil || seen[p] {
			t.Fatalf("phrase %q, %v", p, err)
		}
		seen[p] = true
	}
}

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeStation answers /job from a script and records /status.
type fakeStation struct {
	mu      sync.Mutex
	answers []int // status codes for /job, in turn; the last repeats
	got     []stationStatus
}

func (f *fakeStation) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /job", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		code := f.answers[0]
		if len(f.answers) > 1 {
			f.answers = f.answers[1:]
		}
		f.mu.Unlock()
		w.WriteHeader(code)
		switch code {
		case http.StatusOK:
			_ = json.NewEncoder(w).Encode(handout{MAC: "aa:bb:cc:00:00:01", Tag: "laptop-1", Rev: "abc123",
				Credential: "cred-station", Console: "https://console.example.test"})
		case http.StatusNotFound:
			_, _ = w.Write([]byte(`{"mac":"aa:bb:cc:00:00:01"}`))
		}
	})
	mux.HandleFunc("POST /status", func(w http.ResponseWriter, r *http.Request) {
		var s stationStatus
		_ = json.NewDecoder(r.Body).Decode(&s)
		f.mu.Lock()
		f.got = append(f.got, s)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func stationRig(t *testing.T, f *fakeStation) *rig {
	t.Helper()
	r := newRig(t, "")
	ts := f.serve(t)
	r.station = &stationClient{url: ts.URL, http: ts.Client(), wait: func(context.Context) error { return nil }}
	return r
}

// From a station: wait for approval, then install the approved revision
// without a key, and tell the console every step.
func TestFromAStationTheMachineInstallsWhatTheConsoleApproved(t *testing.T) {
	f := &fakeStation{answers: []int{http.StatusNotFound, http.StatusNotFound, http.StatusOK}}
	r := stationRig(t, f)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, r.out.String())
	}
	out := r.out.String()
	if strings.Count(out, "This machine is aa:bb:cc:00:00:01.") != 1 {
		t.Errorf("waiting shown as:\n%s", out)
	}
	if !strings.Contains(out, "This machine becomes laptop-1.") || strings.Contains(out, "Go on?") {
		t.Errorf("a station install must not wait for a key:\n%s", out)
	}
	if r.called("sextant-agent join") != "" {
		t.Error("a station install asked for a code")
	}
	if c := r.called("git clone"); !strings.Contains(c, "--no-checkout https://console.example.test/fleet.git") {
		t.Errorf("clone %q", c)
	}
	if c := r.called("git -C"); !strings.HasSuffix(c, "checkout --detach abc123") {
		t.Errorf("checkout %q, want the job's revision", c)
	}
	if !strings.Contains(r.called("disko-install"), "#laptop-1") {
		t.Errorf("installed %q", r.called("disko-install"))
	}
	state := filepath.Join(handoverDir, agentStateDir)
	if r.file(t, filepath.Join(state, "credential")) != "cred-station" || !strings.Contains(r.file(t, filepath.Join(state, "joined.json")), `"url":"https://console.example.test"`) {
		t.Error("the station's credential and console are not handed over")
	}

	steps := make([]string, 0, len(f.got))
	for _, s := range f.got {
		steps = append(steps, s.Step+s.Status)
	}
	if got := strings.Join(steps, ","); got != "Fetching the fleet configuration,Installing the system,Binding the disk to the TPM2,Handing the disk phrase to the console,installed" {
		t.Errorf("the console saw %s", got)
	}
	if r.calls[len(r.calls)-1] != "systemctl reboot" {
		t.Errorf("last call %q", r.calls[len(r.calls)-1])
	}
}

func TestAFailedStationInstallSaysWhyInTheConsole(t *testing.T) {
	f := &fakeStation{answers: []int{http.StatusOK}}
	r := stationRig(t, f)
	r.answer["disko"] = runResult{Out: "lots of output\nerror: no space left on device", Code: 1}
	if err := r.Run(context.Background()); err == nil {
		t.Fatal("a failed install returned no error")
	}
	last := f.got[len(f.got)-1]
	if last.Status != "failed" || !strings.Contains(last.Message, "no space left on device") {
		t.Fatalf("the console saw %+v", last)
	}
	if r.called("systemctl reboot") != "" {
		t.Error("restarted after a failure")
	}
}

// At a station nobody can write the phrase down, so an unconfirmed phrase
// fails the install instead of restarting into a prompt nobody can answer,
// and the phrase never appears on the unattended screen.
func TestAtAStationAnUnconfirmedPhraseFailsTheInstall(t *testing.T) {
	f := &fakeStation{answers: []int{http.StatusOK}}
	r := stationRig(t, f)
	r.escrowFails = true
	if err := r.Run(context.Background()); err == nil {
		t.Fatal("installed without the console holding the phrase")
	}
	if last := f.got[len(f.got)-1]; last.Status != "failed" || !strings.Contains(last.Message, "disk phrase") {
		t.Fatalf("the console saw %+v", last)
	}
	if r.escrowed == "" || strings.Contains(r.out.String(), r.escrowed) {
		t.Error("the phrase was shown on an unattended screen")
	}
	if r.called("systemctl reboot") != "" {
		t.Error("restarted without the phrase escrowed")
	}
}

func TestAJobHandedOutAlreadyStops(t *testing.T) {
	r := stationRig(t, &fakeStation{answers: []int{http.StatusGone}})
	if err := r.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "send it again") {
		t.Fatalf("got %v", err)
	}
	if r.called("disko") != "" || r.called("lsblk") != "" {
		t.Error("touched the disks without a job")
	}
}

func TestTheStationComesFromTheKernelCommandLine(t *testing.T) {
	line := "initrd=initrd init=/nix/store/x-init sextant.station=http://10.66.0.1:8080 quiet\n"
	if got := fromCmdline(line, "sextant.station"); got != "http://10.66.0.1:8080" {
		t.Fatalf("got %q", got)
	}
	if got := fromCmdline("quiet sextant.stationx=1", "sextant.station"); got != "" {
		t.Fatalf("a longer key matched: %q", got)
	}
}

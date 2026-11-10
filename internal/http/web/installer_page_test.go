package web_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/web"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// fakeRunner is a gate-runner that builds images instantly or not at all.
type fakeRunner struct {
	mu     sync.Mutex
	asked  []string
	phase  ports.BuildPhase
	detail string
}

func (f *fakeRunner) EnsureArtifact(_ context.Context, rev, name string) (ports.ArtifactState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, rev+"/"+name)
	st := ports.ArtifactState{Phase: f.phase, Detail: f.detail}
	if f.phase == ports.BuildDone {
		st.File, st.Size = "sextant-installer.iso", int64(len("an image"))
	}
	return st, nil
}

func (f *fakeRunner) OpenArtifact(_ context.Context, rev, name string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("an image")), nil
}

func getUSBPage(t *testing.T, u string) (int, string, http.Header) {
	t.Helper()
	resp, err := client().Get(u)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Header
}

// Without a runner the page offers no stick and keeps sending people to
// the command; with one it offers the stick first.
func TestAddALaptopOffersTheUSBOnlyWithARunner(t *testing.T) {
	ts, _, _, _ := joinConsole(t, true)
	_, page, _ := getUSBPage(t, ts.URL+"/devices/add")
	if strings.Contains(page, `id="usb"`) || strings.Contains(page, "installation USB") {
		t.Error("a console without a runner offers a USB stick")
	}
	ts, _, _, _ = joinConsoleWith(t, true, seedFleet, web.DevSessions{}, true, &fakeRunner{phase: ports.BuildBuilding})
	_, page, _ = getUSBPage(t, ts.URL+"/devices/add")
	if !strings.Contains(page, `id="usb"`) || !strings.Contains(page, "Download the installation USB") {
		t.Error("a console with a runner does not offer the stick")
	}
	// A code just made says to start from the stick first.
	_, page, _ = postPage(t, ts.URL+"/devices/add", url.Values{"group": {"pilot"}})
	if !strings.Contains(page, "Start the laptop from the installation USB") {
		t.Error("the code does not say to start from the stick")
	}
}

// The button asks for the stick at the fleet's current revision; the page
// then says how far it is, and once done the image comes through the
// console.
func TestTheUSBIsBuiltAtTheCurrentRevisionAndDownloaded(t *testing.T) {
	runner := &fakeRunner{phase: ports.BuildBuilding}
	ts, _, _, cfg := joinConsoleWith(t, true, seedFleet, web.DevSessions{}, true, runner)
	head := cfg.Head(context.Background())

	code, _, hdr := postPage(t, ts.URL+"/devices/add/usb", url.Values{})
	if code != http.StatusSeeOther || hdr.Get("Location") != "/devices/add?usb="+head+"#usb" {
		t.Fatalf("POST = %d to %q", code, hdr.Get("Location"))
	}
	if len(runner.asked) != 1 || runner.asked[0] != head+"/installer" {
		t.Fatalf("runner asked %v, want %s/installer", runner.asked, head)
	}
	_, page, _ := getUSBPage(t, ts.URL+"/devices/add?usb="+head)
	if !strings.Contains(page, "about ten minutes") {
		t.Error("a stick being built does not say so")
	}
	if code, _, _ := getUSBPage(t, ts.URL+"/devices/add/usb/"+head); code != http.StatusNotFound {
		t.Errorf("download while building = %d, want 404", code)
	}

	runner.phase = ports.BuildDone
	_, page, _ = getUSBPage(t, ts.URL+"/devices/add?usb="+head)
	if !strings.Contains(page, "/devices/add/usb/"+head) || !strings.Contains(page, "sextant-installer.iso") {
		t.Error("a finished stick has no download")
	}
	code, body, hdr := getUSBPage(t, ts.URL+"/devices/add/usb/"+head)
	if code != 200 || body != "an image" || !strings.Contains(hdr.Get("Content-Disposition"), "sextant-installer.iso") {
		t.Errorf("download = %d %q %q", code, body, hdr.Get("Content-Disposition"))
	}
	if code, _, _ := getUSBPage(t, ts.URL+"/devices/add/usb/not-a-rev"); code != http.StatusNotFound {
		t.Errorf("a malformed revision = %d", code)
	}
}

// A fleet without the installer output is told the one line to add.
func TestAFailedUSBSaysWhyAndWhatToAdd(t *testing.T) {
	runner := &fakeRunner{phase: ports.BuildFailed, detail: "flake does not provide attribute 'packages.x86_64-linux.installer'"}
	ts, _, _, cfg := joinConsoleWith(t, true, seedFleet, web.DevSessions{}, true, runner)
	_, page, _ := getUSBPage(t, ts.URL+"/devices/add?usb="+cfg.Head(context.Background()))
	for _, want := range []string{"could not be built", "does not provide attribute", "sextant.lib.mkInstaller"} {
		if !strings.Contains(page, want) {
			t.Errorf("failed stick page lacks %q", want)
		}
	}
}

// Somebody who may not add a laptop gets no stick: it installs laptops into
// the fleet as soon as a code is typed.
func TestTheUSBIsForWhoeverMayAddALaptop(t *testing.T) {
	// A viewer: reaches the page, may not add a laptop.
	viewer := identity.User{Subject: "v", Name: "Beta Viewer", Groups: []string{"beta-view"}}
	runner := &fakeRunner{phase: ports.BuildDone}
	ts, _, _, cfg := joinConsoleWith(t, true, joinScopedSeed, scopedSessions{viewer}, true, runner)
	head := cfg.Head(context.Background())
	if code, _, _ := postPage(t, ts.URL+"/devices/add/usb", url.Values{"csrf": {"csrf"}}); code == http.StatusSeeOther {
		t.Error("somebody without rights started a stick")
	}
	if code, body, _ := getUSBPage(t, ts.URL+"/devices/add/usb/"+head); code != http.StatusForbidden || body == "an image" {
		t.Errorf("somebody without rights downloaded it: %d", code)
	}
	if len(runner.asked) != 0 {
		t.Errorf("runner asked %v for somebody without rights", runner.asked)
	}
}

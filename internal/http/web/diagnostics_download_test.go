package web_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/web"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/secretbox"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// diagnostics_download_test.go: handing a diagnostics bundle to a person.
//
// A bundle is journals and system state from somebody's laptop. The DPIA
// calls it the sharpest personal-data surface the product has, and the
// handler's own comment names the two things that make it defensible:
// editor reach at that device, and a log line naming who took it. Neither
// was covered.

// memDiag is a ports.DiagnosticsStore holding one sealed bundle.
type memDiag struct {
	sealed []byte
	meta   ports.DiagnosticsMeta
	ok     bool
}

func (m *memDiag) Put(context.Context, string, string, []byte, time.Time) error { return nil }

func (m *memDiag) Get(context.Context, string, string) ([]byte, ports.DiagnosticsMeta, bool, error) {
	return m.sealed, m.meta, m.ok, nil
}

func (m *memDiag) Meta(context.Context, string, string) (ports.DiagnosticsMeta, bool, error) {
	return m.meta, m.ok, nil
}

func (m *memDiag) Delete(context.Context, string, string) error { return nil }

// consoleWithDiagnostics wires a console that holds one bundle for lt-1, and
// returns the buffer its logger writes to so the audit line can be read back.
func consoleWithDiagnostics(t *testing.T, store ports.DiagnosticsStore) (*httptest.Server, *bytes.Buffer) {
	t.Helper()
	cfg := seedConsoleConfig(t, ports.GateFunc(func(context.Context, string, []string) error { return nil }))
	sealer, err := secretbox.New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("k"), 32)))
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	diag := app.NewDiagnosticsService(store, sealer, fixedClock{time.Now().UTC()}, "")
	srv, err := web.New(web.Services{Config: cfg, Diagnostics: diag}, web.DevSessions{}, true,
		nil, nil, nil, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	srv.Routes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, &logs
}

// sealedFor produces what the store would hold for a bundle.
func sealedFor(t *testing.T, plain string) []byte {
	t.Helper()
	sealer, err := secretbox.New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("k"), 32)))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := sealer.Seal([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func TestDownloadingABundleIsLoggedWithWhoTookIt(t *testing.T) {
	created := time.Date(2026, 9, 9, 8, 0, 0, 0, time.UTC)
	store := &memDiag{
		sealed: sealedFor(t, "journalctl output"),
		meta:   ports.DiagnosticsMeta{Created: created},
		ok:     true,
	}
	ts, logs := consoleWithDiagnostics(t, store)

	resp, err := client().Get(ts.URL + "/devices/lt-1/diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download = %d, want 200: %s", resp.StatusCode, body)
	}
	if string(body) != "journalctl output" {
		t.Errorf("body = %q; the bundle was not unsealed", body)
	}
	// Served as a file, not rendered: a gzip through a text/html path arrives
	// corrupted and the operator blames the device.
	if ct := resp.Header.Get("Content-Type"); ct != "application/gzip" {
		t.Errorf("content-type = %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "lt-1-diagnostics-2026-09-09.gz") {
		t.Errorf("content-disposition = %q; the date names the bundle, not today", cd)
	}

	// The accountability half. A download that leaves no trace is the finding
	// the DPIA raises about this surface, so the line has to name both the
	// device and the person.
	line := logs.String()
	if !strings.Contains(line, "diagnostics bundle downloaded") {
		t.Fatalf("the download was not logged at all: %s", line)
	}
	for _, want := range []string{"lt-1", "dev"} {
		if !strings.Contains(line, want) {
			t.Errorf("the audit line does not name %q: %s", want, line)
		}
	}
}

func TestABundleThatIsNotThereIsNotFound(t *testing.T) {
	ts, _ := consoleWithDiagnostics(t, &memDiag{ok: false})
	resp, err := client().Get(ts.URL + "/devices/lt-1/diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("missing bundle = %d, want 404", resp.StatusCode)
	}
}

// TestDiagnosticsDisabledSaysSoRatherThanPanicking: Diagnostics is nil when
// the deployment turned it off or has no database, and this handler
// dereferences it.
func TestDiagnosticsDisabledSaysSoRatherThanPanicking(t *testing.T) {
	ts, _ := newConsole(t)
	resp, err := client().Get(ts.URL + "/devices/lt-1/diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("disabled = %d, want 404", resp.StatusCode)
	}
	if !strings.Contains(string(body), "disabled") {
		t.Errorf("the answer does not say it is disabled: %s", body)
	}
}

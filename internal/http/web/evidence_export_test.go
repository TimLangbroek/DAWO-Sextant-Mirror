package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/web"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// evidence_export_test.go: the bundle an auditor is handed.
//
// The handler turns two date inputs into the window the export covers, and
// nothing checked that arithmetic. A window that quietly shifts by a day
// produces a bundle that looks complete and is missing the day somebody
// asked about, which is the worst shape a compliance answer can take: wrong
// without looking wrong.
//
// The window is observable without a fake: the filename carries both ends.

// evidenceSeed grants a viewer at ONE GROUP, which is the interesting
// account here: they are inside the console and must still be refused an
// organisation-wide bundle.
const evidenceSeed = `{
  "version": 3,
  "org": {"settings": {"desktop": "plasma"}},
  "groups": {"pilot": {}},
  "devices": {"lt-1": {"groups": ["pilot"], "hardware": "hw"}},
  "access": [{"group": "pilot-viewers", "role": "viewer", "scope": "group:pilot"}]
}`

func consoleWithEvidence(t *testing.T, u identity.User) *httptest.Server {
	t.Helper()
	cfg := seedConsoleConfigWith(t, ports.GateFunc(func(context.Context, string, []string) error { return nil }), evidenceSeed)
	ev := app.NewEvidenceService(cfg, nil, app.SystemClock{})
	srv, err := web.New(web.Services{Config: cfg, Evidence: ev}, scopedSessions{u}, true,
		nil, nil, []string{"owners"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	srv.Routes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestTheEvidenceWindowIsTheOneThatWasAskedFor(t *testing.T) {
	ts := consoleWithEvidence(t, identity.User{Subject: "o", Groups: []string{"owners"}})

	resp, err := client().Get(ts.URL + "/audit/evidence?from=2026-08-01&to=2026-08-31")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export = %d: %s", resp.StatusCode, body)
	}
	// The end is bumped a day because the window is half-open. Ask for the
	// 31st and the 31st has to be inside it.
	const want = "sextant-evidence-20260801-20260901.json"
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, want) {
		t.Errorf("content-disposition = %q, want the window %s", cd, want)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
	if !strings.Contains(string(body), "{") {
		t.Errorf("the bundle is not JSON: %.100s", body)
	}
}

// TestAnUnparsableDateFallsBackRatherThanFailing: the form posts whatever the
// browser gives it, and an auditor mistyping a date should get the default
// window, not an error page.
func TestAnUnparsableDateFallsBackRatherThanFailing(t *testing.T) {
	ts := consoleWithEvidence(t, identity.User{Subject: "o", Groups: []string{"owners"}})
	resp, err := client().Get(ts.URL + "/audit/evidence?from=gisteren&to=vandaag")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export with junk dates = %d, want the default window: %s", resp.StatusCode, body)
	}
	// And the default window is a month, not whatever the code happens to
	// say. Somebody asking for "the default" is asking for the last thirty
	// days; a week would answer a different question and look identical.
	cd := resp.Header.Get("Content-Disposition")
	i := strings.Index(cd, "sextant-evidence-")
	if i < 0 {
		t.Fatalf("content-disposition = %q", cd)
	}
	window := cd[i+len("sextant-evidence-"):]
	window = strings.TrimSuffix(strings.TrimSuffix(window, `"`), ".json")
	window = strings.TrimSuffix(window, ".json")
	parts := strings.Split(strings.TrimSuffix(window, `"`), "-")
	if len(parts) != 2 {
		t.Fatalf("cannot read the window out of %q", cd)
	}
	from, err1 := time.Parse("20060102", parts[0])
	to, err2 := time.Parse("20060102", parts[1])
	if err1 != nil || err2 != nil {
		t.Fatalf("cannot parse the window %q: %v %v", window, err1, err2)
	}
	if days := int(to.Sub(from).Hours() / 24); days != 30 {
		t.Errorf("default window = %d days, want 30", days)
	}
}

// TestOnlyAnOrgViewerGetsTheEvidence: the bundle carries who approved what
// across the whole organisation, so a grant on one group is not enough.
func TestOnlyAnOrgViewerGetsTheEvidence(t *testing.T) {
	ts := consoleWithEvidence(t, identity.User{Subject: "u", Groups: []string{"pilot-viewers"}})
	resp, err := client().Get(ts.URL + "/audit/evidence?from=2026-08-01&to=2026-08-31")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Error("a group-scoped account downloaded the organisation's evidence bundle")
	}
}

// TestEvidenceUnavailableSaysSo: Evidence is nil when the config plane is not
// mounted, and the handler dereferences it.
func TestEvidenceUnavailableSaysSo(t *testing.T) {
	ts, _ := newConsole(t)
	resp, err := client().Get(ts.URL + "/audit/evidence")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unavailable = %d, want 404", resp.StatusCode)
	}
	if !strings.Contains(string(body), "unavailable") {
		t.Errorf("the answer does not say why: %s", body)
	}
}

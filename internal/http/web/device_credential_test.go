package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/token"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/web"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// device_credential_test.go: re-issuing a device credential from the console.
//
// The handler sat at 0%. It mints a secret that lets a machine speak for a
// device for five years, and the only thing standing between that and a
// retired laptop is one branch. A credential issued to a device somebody
// decided to decommission re-arms it, quietly, and the console reports
// success.

// consoleWithCredentials is the seeded console plus a device-credential
// service, which newConsole deliberately leaves out.
func consoleWithCredentials(t *testing.T) (*httptest.Server, *app.ConfigService, *memTokens) {
	t.Helper()
	cfg := seedConsoleConfig(t, ports.GateFunc(func(context.Context, string, []string) error { return nil }))
	store := &memTokens{toks: map[string]token.Token{}}
	creds := app.NewDeviceCredentials(store, fixedClock{time.Unix(1, 0).UTC()})
	srv, err := web.New(web.Services{Config: cfg, DevCreds: creds}, web.DevSessions{}, true,
		nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	srv.Routes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, cfg, store
}

func TestReissuingADeviceCredential(t *testing.T) {
	ts, cfg, store := consoleWithCredentials(t)
	post := func(path string) *http.Response {
		t.Helper()
		resp, err := client().PostForm(ts.URL+path, url.Values{"csrf": {"dev-csrf"}})
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	resp := post("/devices/lt-1/credential")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("issue = %d, want 303: %s", resp.StatusCode, body)
	}
	if _, ok, _ := store.Get(context.Background(), "device-lt-1"); !ok {
		t.Error("no credential was stored for the device")
	}
	// The secret leaves through the one-shot cookie, never the redirect.
	var carried string
	for _, c := range resp.Cookies() {
		if strings.Contains(c.Name, "lt-1") || strings.Contains(c.Name, "devcred") {
			carried = c.Value
		}
	}
	if carried == "" {
		t.Error("the console issued a credential and handed the operator nothing")
	}
	if loc := resp.Header.Get("Location"); strings.Contains(loc, carried) && carried != "" {
		t.Errorf("the secret is in the redirect URL: %s", loc)
	}

	// A retired device is refused. Re-issuing here re-arms a machine somebody
	// decided to decommission.
	if r := post("/devices/lt-1/retire"); r.StatusCode != http.StatusSeeOther {
		r.Body.Close()
		t.Fatalf("retire = %d", r.StatusCode)
	}
	if !cfg.Fleet().Devices["lt-1"].Retired() {
		t.Fatal("fixture: the device is not retired")
	}
	r2 := post("/devices/lt-1/credential")
	body, _ := io.ReadAll(r2.Body)
	r2.Body.Close()
	if r2.StatusCode == http.StatusSeeOther {
		t.Error("a retired device was handed a fresh credential")
	}
	if !strings.Contains(strings.ToLower(string(body)), "retired") {
		t.Errorf("the refusal does not say the device is retired: %s", body)
	}
}

// TestADeviceThatDoesNotExistGetsNoCredential: the tag comes from the URL, so
// it is whatever somebody typed.
func TestADeviceThatDoesNotExistGetsNoCredential(t *testing.T) {
	ts, _, store := consoleWithCredentials(t)
	resp, err := client().PostForm(ts.URL+"/devices/lt-nope/credential", url.Values{"csrf": {"dev-csrf"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusSeeOther {
		t.Error("a credential was issued for a device that does not exist")
	}
	if _, ok, _ := store.Get(context.Background(), "device-lt-nope"); ok {
		t.Error("a credential was stored under a tag nothing knows")
	}
}

// TestWithoutADatabaseTheConsoleSaysSoRatherThanPanicking: DevCreds is nil on
// a console without postgres, and this handler dereferences it.
func TestWithoutADatabaseTheConsoleSaysSoRatherThanPanicking(t *testing.T) {
	ts, _ := newConsole(t)
	resp, err := client().PostForm(ts.URL+"/devices/lt-1/credential", url.Values{"csrf": {"dev-csrf"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode == http.StatusSeeOther {
		t.Fatal("a console with no credential store reported success")
	}
	if !strings.Contains(strings.ToLower(string(body)), "postgres") {
		t.Errorf("the error does not name what is missing: %s", body)
	}
}

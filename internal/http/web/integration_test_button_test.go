package web_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/observed"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/web"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

func netbird(state, detail string) observed.Integrations {
	return observed.Integrations{"netbird": {State: state, Detail: detail}}
}

// testConsole: NetBird on for the organisation with managementUrl, five
// devices of which one is retired and one is in a group that turned it off.
func testConsole(t *testing.T, managementURL string, sessions web.Sessions) *httptest.Server {
	t.Helper()
	doc := fmt.Sprintf(`{
  "version": 3,
  "org": {"settings": {"netbird.enable": true, "netbird.managementUrl": %q}},
  "groups": {"pilot": {}, "staff": {"settings": {"netbird.enable": false}}},
  "devices": {
    "lt-up":   {"groups": ["pilot"], "hardware": "hw"},
    "lt-down": {"groups": ["pilot"], "hardware": "hw"},
    "lt-new":  {"groups": ["pilot"], "hardware": "hw"},
    "lt-old":  {"groups": ["pilot"], "hardware": "hw", "state": "retired"},
    "lt-staff": {"groups": ["staff"], "hardware": "hw"}
  },
  "access": [{"group": "pilot-view", "role": "viewer", "scope": "org"}]
}`, managementURL)
	cfg := seedConsoleConfigWith(t, ports.GateFunc(func(context.Context, string, []string) error { return nil }), doc)
	now := time.Now()
	status := &manyStatus{sts: []observed.DeviceStatus{
		{Tag: "lt-up", LastSeen: now, Integrations: netbird("up", "on the mesh")},
		{Tag: "lt-down", LastSeen: now, Integrations: netbird("down", "netbird-dawo.service failed: invalid setup key")},
		{Tag: "lt-old", LastSeen: now, Integrations: netbird("down", "retired, must not count")},
		{Tag: "lt-staff", LastSeen: now, Integrations: netbird("up", "")},
	}}
	inv := app.NewInventoryService(status, noFacts{}, fixedClock{now}, app.DefaultTenant)
	srv, err := web.New(web.Services{Config: cfg, Inventory: inv}, sessions, true,
		nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	srv.Routes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func TestTestingAnIntegrationAsksTheServerAndTheDevices(t *testing.T) {
	mgmt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer mgmt.Close()
	ts := testConsole(t, mgmt.URL, web.DevSessions{})

	if page := getBody(t, ts.URL+"/integrations"); !strings.Contains(page, `action="/integrations/netbird/test"`) {
		t.Fatal("the NetBird card has no Test button")
	}
	status, page, _ := postPage(t, ts.URL+"/integrations/netbird/test", url.Values{"scope": {"org"}})
	if status != http.StatusOK {
		t.Fatalf("test = %d\n%s", status, page)
	}
	if !strings.Contains(page, `data-integration-result="netbird"`) || strings.Contains(page, `data-integration-result="wazuh"`) {
		t.Fatal("the answer is not on the NetBird card alone")
	}
	if !strings.Contains(page, "<code>"+mgmt.URL+"</code> HTTP 200 OK") {
		t.Fatalf("the server's answer is missing:\n%s", page)
	}
	// Retired devices and devices that have it off do not count;
	// unmeasured is its own count.
	if !strings.Contains(page, "1 up · 1 down · 1 no reading yet") {
		t.Fatalf("device counts:\n%s", page)
	}
	if !strings.Contains(page, `<a href="/devices/lt-down" class="font-mono">lt-down</a>: netbird-dawo.service failed: invalid setup key`) {
		t.Fatal("the down device's own words are missing")
	}

	// A group sees its own devices only, and staff turned it off.
	_, page, _ = postPage(t, ts.URL+"/integrations/netbird/test", url.Values{"scope": {"group:pilot"}})
	if !strings.Contains(page, "1 up · 1 down · 1 no reading yet") {
		t.Fatalf("pilot counts:\n%s", page)
	}
	_, page, _ = postPage(t, ts.URL+"/integrations/netbird/test", url.Values{"scope": {"group:staff"}})
	if !strings.Contains(page, "No device in this scope has it on yet.") {
		t.Fatalf("staff counts:\n%s", page)
	}
}

func TestAnUnreachableServerIsSaidToBe(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	addr := dead.URL
	dead.Close()
	ts := testConsole(t, addr, web.DevSessions{})
	_, page, _ := postPage(t, ts.URL+"/integrations/netbird/test", url.Values{"scope": {"org"}})
	if !strings.Contains(page, `text-status-error">error</span>`) || strings.Contains(page, "HTTP 200") {
		t.Fatalf("an unreachable server did not read as one:\n%s", page)
	}
}

func TestOnlyAnEditorTests(t *testing.T) {
	viewer := identity.User{Subject: "v", Name: "Viewer", Groups: []string{"pilot-view"}}
	var hits atomic.Int32
	mgmt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1) }))
	defer mgmt.Close()
	ts := testConsole(t, mgmt.URL, scopedSessions{viewer})
	if page := getBody(t, ts.URL+"/integrations"); strings.Contains(page, "/integrations/netbird/test") {
		t.Error("a viewer is offered Test")
	}
	postPage(t, ts.URL+"/integrations/netbird/test", url.Values{"csrf": {"csrf"}, "scope": {"org"}})
	if hits.Load() != 0 {
		t.Error("a viewer made the console contact the server")
	}
}

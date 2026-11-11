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

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/discovery"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/web"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

func pagesConsole(t *testing.T, doc string, prefs ports.PrefsStore) *httptest.Server {
	t.Helper()
	cfg := seedConsoleConfigWith(t, ports.GateFunc(func(context.Context, string, []string) error { return nil }), doc)
	svc := web.Services{Config: cfg,
		Discovery: app.NewDiscoveryService(&memDisc{sets: map[string][]discovery.Discovered{}}, clockNow{}, "")}
	if prefs != nil {
		svc.Prefs = prefs
	}
	srv, err := web.New(svc, web.DevSessions{}, true, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	srv.Routes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

func navHas(page, href string) bool {
	return strings.Contains(page, `<a href="`+href+`" class="nav-link`)
}

// Simple by default (design 0020): the bar shows the everyday pages, the
// switch shows the rest, and hiding is never a refusal.
func TestTheSidebarIsSimpleUntilYouAskForEveryPage(t *testing.T) {
	prefs := &memPrefs{m: map[string]identity.Preferences{}}
	ts := pagesConsole(t, seedFleet, prefs)

	page := getBody(t, ts.URL+"/devices")
	for _, href := range []string{"/", "/devices", "/groups", "/settings", "/integrations", "/updates"} {
		if !navHas(page, href) {
			t.Errorf("simple bar lacks %s", href)
		}
	}
	for _, href := range []string{"/policies", "/hardware", "/compliance", "/enroll"} {
		if navHas(page, href) {
			t.Errorf("simple bar shows %s", href)
		}
	}
	if !strings.Contains(page, `data-all-pages="off"`) {
		t.Fatal("no switch to show every page")
	}

	// A hidden page is still there, and in the bar while it is being read.
	resp, err := client().Get(ts.URL + "/policies")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !navHas(string(body), "/policies") || navHas(string(body), "/hardware") {
		t.Fatalf("policies = %d; in the bar: %v", resp.StatusCode, navHas(string(body), "/policies"))
	}

	// The switch returns to the page it was pressed on.
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/profile/all-pages",
		strings.NewReader(url.Values{"csrf": {"dev-csrf"}, "all": {"on"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", ts.URL+"/devices?group=pilot")
	resp, err = client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/devices?group=pilot" {
		t.Fatalf("switch = %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	page = getBody(t, ts.URL+"/devices")
	for _, href := range []string{"/policies", "/hardware", "/compliance", "/enroll"} {
		if !navHas(page, href) {
			t.Errorf("full bar lacks %s", href)
		}
	}

	// Saving the time zone keeps the choice.
	if status, _, _ := postPage(t, ts.URL+"/profile/prefs", url.Values{"timezone": {"Europe/Amsterdam"}, "locale": {"en"}}); status != http.StatusSeeOther {
		t.Fatalf("prefs save = %d", status)
	}
	if !navHas(getBody(t, ts.URL+"/devices"), "/policies") {
		t.Fatal("saving preferences turned the full bar off")
	}
}

func TestTheSwitchNeverSendsYouElsewhere(t *testing.T) {
	ts := pagesConsole(t, seedFleet, &memPrefs{m: map[string]identity.Preferences{}})
	for _, ref := range []string{"https://evil.example/devices", "", "not a url"} {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/profile/all-pages",
			strings.NewReader(url.Values{"csrf": {"dev-csrf"}, "all": {"on"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if ref != "" {
			req.Header.Set("Referer", ref)
		}
		resp, err := client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if loc := resp.Header.Get("Location"); loc != "/" {
			t.Errorf("Referer %q sent to %q", ref, loc)
		}
	}
}

func TestEnrollmentIsInTheSimpleBarOnceThereIsAStation(t *testing.T) {
	doc := `{"version": 3, "groups": {"pilot": {}}, "stations": {"nuc-1": {}}}`
	ts := pagesConsole(t, doc, &memPrefs{m: map[string]identity.Preferences{}})
	if !navHas(getBody(t, ts.URL+"/devices"), "/enroll") {
		t.Fatal("an organisation with a station does not see Enrollment")
	}
}

// Without a database there is nowhere to keep the choice, so every page.
func TestWithoutPreferencesEveryPageShows(t *testing.T) {
	ts := pagesConsole(t, seedFleet, nil)
	page := getBody(t, ts.URL+"/devices")
	if !navHas(page, "/policies") || strings.Contains(page, "data-all-pages") {
		t.Fatal("without preferences the bar hides pages or offers a switch it cannot keep")
	}
}

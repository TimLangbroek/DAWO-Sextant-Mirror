package web_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/adapters/git"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/joincode"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/token"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/web"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

type memCodes struct {
	mu sync.Mutex
	m  map[string]joincode.Code
}

func (s *memCodes) Create(_ context.Context, _ string, c joincode.Code) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[c.ID] = c
	return nil
}

func (s *memCodes) Spend(_ context.Context, _ string, hash []byte, now time.Time) (joincode.Code, bool, error) {
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

func (s *memCodes) Refund(_ context.Context, _, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.m[id]
	c.Uses++
	s.m[id] = c
	return nil
}

func (s *memCodes) Open(_ context.Context, _ string, now time.Time) ([]joincode.Code, error) {
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

func (s *memCodes) Withdraw(_ context.Context, _, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.m[id]
	c.Uses = 0
	s.m[id] = c
	return nil
}

// joinConsole is a console with join codes when withJoins is set, and
// without (no database) otherwise.
func joinConsole(t *testing.T, withJoins bool) (*httptest.Server, *app.JoinService, *memCodes, *app.ConfigService) {
	t.Helper()
	return joinConsoleAs(t, withJoins, seedFleet, web.DevSessions{})
}

func joinConsoleAs(t *testing.T, withJoins bool, seed string, sessions web.Sessions) (*httptest.Server, *app.JoinService, *memCodes, *app.ConfigService) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	for name, body := range map[string]string{"fleet.json": seed, "catalog.json": seedCatalog,
		"hardware-profiles.json": `[{"name":"hw"},{"name":"generic-intel"}]`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", ".")
	run("-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "seed")
	repo, err := git.Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := app.NewConfigService(repo, ports.GateFunc(func(context.Context, string, []string) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	svc := web.Services{Config: cfg}
	codes := &memCodes{m: map[string]joincode.Code{}}
	var joins *app.JoinService
	if withJoins {
		clock := fixedClock{time.Now()}
		creds := app.NewDeviceCredentials(&memTokens{toks: map[string]token.Token{}}, clock)
		joins = app.NewJoinService(codes, cfg, creds, clock, app.DefaultTenant, []byte("session key"))
		svc.DevCreds, svc.Joins = creds, joins
		svc.JoinConsole = func() (string, string) { return "https://console.example.test", "main" }
	}
	srv, err := web.New(svc, sessions, true, nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	srv.Routes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, joins, codes, cfg
}

var shownCodeRE = regexp.MustCompile(`data-join-code>([0-9A-Z]{4}-[0-9A-Z]{4})<`)

func postPage(t *testing.T, u string, form url.Values) (int, string, http.Header) {
	t.Helper()
	if form.Get("csrf") == "" {
		form.Set("csrf", "dev-csrf")
	}
	resp, err := client().PostForm(u, form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), resp.Header
}

func TestAddALaptopShowsACodeThatWorksOnce(t *testing.T) {
	ts, joins, _, cfg := joinConsole(t, true)

	resp, err := client().Get(ts.URL + "/devices")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `href="/devices/add"`) {
		t.Fatal("the sidebar does not offer Add a laptop")
	}

	status, page, hdr := postPage(t, ts.URL+"/devices/add", url.Values{"group": {"pilot"}})
	if status != http.StatusOK {
		t.Fatalf("make a code = %d\n%s", status, page)
	}
	if hdr.Get("Cache-Control") != "no-store" {
		t.Fatal("the page with the code may be cached")
	}
	m := shownCodeRE.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no code on the page:\n%s", page)
	}
	code := m[1]
	if !strings.Contains(page, "sudo sextant-agent join https://console.example.test "+code) {
		t.Fatal("the page does not show the command to run")
	}

	got, err := joins.Join(context.Background(), app.JoinRequest{Code: code, Serial: "PF3ABCDE", CPU: "GenuineIntel"})
	if err != nil {
		t.Fatalf("the shown code does not join: %v", err)
	}
	if g := cfg.Fleet().Devices[got.Tag].Groups; len(g) != 1 || g[0] != "pilot" {
		t.Fatalf("joined into %v", g)
	}
	if _, err := joins.Join(context.Background(), app.JoinRequest{Code: code, Serial: "OTHER"}); err == nil {
		t.Fatal("a one-laptop code joined twice")
	}
}

func TestOpenCodesAreListedAndCanBeWithdrawn(t *testing.T) {
	ts, joins, codes, _ := joinConsole(t, true)
	_, page, _ := postPage(t, ts.URL+"/devices/add", url.Values{"group": {"pilot"}, "uses": {"3"}})
	code := shownCodeRE.FindStringSubmatch(page)[1]

	resp, err := client().Get(ts.URL + "/devices/add")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	list := string(body)
	if strings.Contains(list, code) {
		t.Fatal("the code is shown again: it must exist only in the answer that made it")
	}
	var id string
	for k := range codes.m {
		id = k
	}
	if !strings.Contains(list, "/devices/add/"+id+"/withdraw") || !strings.Contains(list, "<td>3</td>") {
		t.Fatalf("the open code is not listed with its three laptops:\n%s", list)
	}

	if status, _, _ := postPage(t, ts.URL+"/devices/add/"+id+"/withdraw", url.Values{}); status != http.StatusSeeOther {
		t.Fatalf("withdraw = %d", status)
	}
	if _, err := joins.Join(context.Background(), app.JoinRequest{Code: code, Serial: "S1", CPU: "GenuineIntel"}); err == nil {
		t.Fatal("a withdrawn code joined")
	}
}

func TestReenrolMakesACodeForThatDevice(t *testing.T) {
	ts, joins, _, _ := joinConsole(t, true)
	resp, err := client().Get(ts.URL + "/devices/lt-1")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), `action="/devices/lt-1/reenrol"`) {
		t.Fatal("the device page has no Re-enrol")
	}
	status, page, _ := postPage(t, ts.URL+"/devices/lt-1/reenrol", url.Values{})
	m := shownCodeRE.FindStringSubmatch(page)
	if status != http.StatusOK || m == nil || !strings.Contains(page, "lt-1") {
		t.Fatalf("re-enrol = %d\n%s", status, page)
	}
	got, err := joins.Join(context.Background(), app.JoinRequest{Code: m[1], Serial: "ANY"})
	if err != nil || got.Tag != "lt-1" {
		t.Fatalf("the re-enrol code: %+v %v", got, err)
	}
}

func TestAddALaptopRefusesABadNumber(t *testing.T) {
	ts, _, codes, _ := joinConsole(t, true)
	for _, uses := range []string{"abc", "0", "101"} {
		status, page, _ := postPage(t, ts.URL+"/devices/add", url.Values{"group": {"pilot"}, "uses": {uses}})
		if status == http.StatusOK || shownCodeRE.MatchString(page) {
			t.Errorf("uses=%s: %d, code shown=%v", uses, status, shownCodeRE.MatchString(page))
		}
	}
	if len(codes.m) != 0 {
		t.Fatalf("refused forms made %d codes", len(codes.m))
	}
}

// Without a database there are no codes. The sidebar must not offer a door
// that only opens on an error, and the page must say why.
func TestAddALaptopWithoutTheDatabase(t *testing.T) {
	ts, _, _, _ := joinConsole(t, false)
	resp, err := client().Get(ts.URL + "/devices")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), `href="/devices/add"`) {
		t.Fatal("the sidebar offers Add a laptop on a console that cannot make codes")
	}
	resp, err = client().Get(ts.URL + "/devices/add")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("/devices/add without a database = %d, want 503 with the reason", resp.StatusCode)
	}
}

const joinScopedSeed = `{
  "version": 3,
  "groups": {"alpha": {}, "beta": {}},
  "devices": {"b-1": {"groups": ["beta"], "hardware": "hw"}},
  "access": [
    {"group": "alpha-eds", "role": "editor", "scope": "group:alpha"},
    {"group": "beta-view", "role": "viewer", "scope": "group:beta"}
  ]
}`

// A code puts a laptop in a group, so making one is editing that group: an
// editor of alpha makes codes for alpha and nothing else, and does not see
// or withdraw another group's codes.
func TestAddALaptopIsScopedToTheGroupsYouEdit(t *testing.T) {
	alpha := identity.User{Subject: "a", Name: "Alpha Editor", Groups: []string{"alpha-eds"}}
	ts, joins, codes, _ := joinConsoleAs(t, true, joinScopedSeed, scopedSessions{alpha})
	ctx := context.Background()
	if _, _, err := joins.Create(ctx, app.NewCode{Group: "beta", By: "someone"}); err != nil {
		t.Fatal(err)
	}
	var betaID string
	for id := range codes.m {
		betaID = id
	}

	resp, err := client().Get(ts.URL + "/devices/add")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	page := string(body)
	if resp.StatusCode != http.StatusOK || !strings.Contains(page, `<option value="alpha">`) {
		t.Fatalf("alpha's editor cannot pick alpha (%d)", resp.StatusCode)
	}
	if strings.Contains(page, `<option value="beta">`) || strings.Contains(page, `<option value="">`) {
		t.Fatal("alpha's editor is offered a group or the organisation they cannot edit")
	}
	if strings.Contains(page, betaID) {
		t.Fatal("alpha's editor sees beta's code")
	}

	for _, group := range []string{"beta", ""} {
		status, page, _ := postPage(t, ts.URL+"/devices/add", url.Values{"group": {group}, "csrf": {"csrf"}})
		if status != http.StatusForbidden || shownCodeRE.MatchString(page) {
			t.Errorf("a code for %q by alpha's editor: %d", group, status)
		}
	}
	if status, _, _ := postPage(t, ts.URL+"/devices/add/"+betaID+"/withdraw", url.Values{"csrf": {"csrf"}}); status != http.StatusForbidden {
		t.Errorf("alpha's editor withdrew beta's code: %d", status)
	}
	if !codes.m[betaID].Open(time.Now()) {
		t.Fatal("beta's code was closed by someone who may not")
	}
	status, page, _ := postPage(t, ts.URL+"/devices/add", url.Values{"group": {"alpha"}, "csrf": {"csrf"}})
	if status != http.StatusOK || !shownCodeRE.MatchString(page) {
		t.Fatalf("alpha's editor cannot make a code for alpha: %d", status)
	}
	if status, _, _ := postPage(t, ts.URL+"/devices/b-1/reenrol", url.Values{"csrf": {"csrf"}}); status != http.StatusForbidden {
		t.Errorf("alpha's editor made a re-enrol code for a beta device: %d", status)
	}

	// Seeing a group is not editing it: someone who edits alpha and views
	// beta still makes no code for beta.
	both := identity.User{Subject: "b", Groups: []string{"alpha-eds", "beta-view"}}
	ts, _, _, _ = joinConsoleAs(t, true, joinScopedSeed, scopedSessions{both})
	if status, _, _ := postPage(t, ts.URL+"/devices/add", url.Values{"group": {"beta"}, "csrf": {"csrf"}}); status != http.StatusForbidden {
		t.Errorf("a viewer of beta made a code for beta: %d", status)
	}
	viewer := identity.User{Subject: "v", Groups: []string{"beta-view"}}
	ts, _, _, _ = joinConsoleAs(t, true, joinScopedSeed, scopedSessions{viewer})
	resp, err = client().Get(ts.URL + "/devices/add")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("a viewer may open Add a laptop: %d", resp.StatusCode)
	}
}

// Add a laptop is the primary button on every page, and a button that ends
// in a refusal is worse than none: whoever cannot make a code does not see
// it, whoever can sees it on the overview and on an empty device list too.
func TestAddALaptopIsOfferedToWhoeverCanUseIt(t *testing.T) {
	get := func(ts *httptest.Server, path string) string {
		t.Helper()
		resp, err := client().Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body)
	}
	const offer = `href="/devices/add"`

	viewer := identity.User{Subject: "v", Groups: []string{"beta-view"}}
	ts, _, _, _ := joinConsoleAs(t, true, joinScopedSeed, scopedSessions{viewer})
	for _, path := range []string{"/", "/devices"} {
		if strings.Contains(get(ts, path), offer) {
			t.Errorf("%s offers Add a laptop to someone who only views beta", path)
		}
	}

	alpha := identity.User{Subject: "a", Groups: []string{"alpha-eds"}}
	ts, _, _, _ = joinConsoleAs(t, true, joinScopedSeed, scopedSessions{alpha})
	if !strings.Contains(get(ts, "/devices"), offer) {
		t.Error("alpha's editor is not offered Add a laptop")
	}

	ts, _, _, _ = joinConsoleAs(t, true, `{"version": 3, "groups": {"pilot": {}}, "devices": {}}`, web.DevSessions{})
	if n := strings.Count(get(ts, "/"), offer); n != 2 {
		t.Errorf("the overview offers Add a laptop %d times, want the sidebar and the quick action", n)
	}
	if n := strings.Count(get(ts, "/devices"), offer); n != 2 {
		t.Errorf("an empty device list offers Add a laptop %d times, want the sidebar and the empty row", n)
	}
}

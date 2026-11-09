package web

import (
	"context"
	"encoding/base64"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/config"
)

type fakeFlow struct {
	user   identity.User
	err    error
	logins int
}

func (f *fakeFlow) Login(w http.ResponseWriter, r *http.Request) {
	f.logins++
	http.Redirect(w, r, "https://id.example.com/authorize", http.StatusFound)
}

func (f *fakeFlow) Identify(http.ResponseWriter, *http.Request) (identity.User, error) {
	return f.user, f.err
}

// signinRig is a setup at the sign-in step with a fake provider.
type signinRig struct {
	mux      *http.ServeMux
	dir      string
	flow     *fakeFlow
	got      []SigninConfig
	checkErr error
	checks   int
	restarts int
}

var testSessionKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func newSigninRig(t *testing.T, mod func(*SetupOptions)) *signinRig {
	t.Helper()
	rig := &signinRig{dir: t.TempDir(), flow: &fakeFlow{user: identity.User{
		Subject: "sub-1", Name: "Setup Person", Email: "setup@example.com", Groups: []string{"admins"}}}}
	keys := config.Keys{SessionKey: testSessionKey, SecretKey: "secret-key", CheckinToken: "checkin"}
	if err := config.WriteKeys(rig.dir, keys); err != nil {
		t.Fatal(err)
	}
	settings := config.Settings{RepoDir: "/data/overlay", OwnerSubjects: []string{"sub-earlier"}}
	if err := config.WriteSettings(rig.dir, settings); err != nil {
		t.Fatal(err)
	}
	opt := SetupOptions{DataDir: rig.dir, Keys: keys, Settings: settings,
		NewSignin: func(_ context.Context, c SigninConfig) (SigninFlow, error) {
			rig.got = append(rig.got, c)
			if strings.Contains(c.Issuer, "down") {
				return nil, errors.New("oidc discovery: connection refused")
			}
			return rig.flow, nil
		},
		Check:   func() error { rig.checks++; return rig.checkErr },
		Restart: func() { rig.restarts++ },
	}
	if mod != nil {
		mod(&opt)
	}
	rig.mux = newTestSetup(t, opt)
	return rig
}

func signinForm(pub, issuer, client, secret string) url.Values {
	return url.Values{"action": {"check"}, "public_url": {pub}, "issuer": {issuer},
		"client_id": {client}, "client_secret": {secret}}
}

func text(key string) string {
	return template.HTMLEscapeString(newLocalizer(identity.Preferences{}, "", "UTC").T(key))
}

func readDir(t *testing.T, dir string) (config.Settings, config.Keys) {
	t.Helper()
	s, k, err := config.ReadDataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s, k
}

// The whole step: check the provider, sign in once, see what came back,
// finish. The person who signed in becomes owner by subject, the answers
// land in the data directory, and the console restarts.
func TestSigninCheckTestFinish(t *testing.T) {
	rig := newSigninRig(t, nil)

	w := do(rig.mux, "GET", "/setup", nil, true)
	if !strings.Contains(w.Body.String(), `value="http://example.com"`) ||
		!strings.Contains(w.Body.String(), "http://example.com/callback") {
		t.Fatalf("the address is not guessed from the browser's request:\n%s", w.Body.String())
	}

	w = do(rig.mux, "POST", "/setup/signin", signinForm("https://sextant.example.org/", "https://id.example.com", "sextant", "s3cret"), true)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `href="/setup/signin/start"`) {
		t.Fatalf("check: %d\n%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "s3cret") {
		t.Fatal("the client secret came back in the page")
	}
	c := rig.got[0]
	if c.RedirectURL != "https://sextant.example.org/callback" || c.ClientSecret != "s3cret" || !c.Secure || len(c.SessionKey) != 32 {
		t.Fatalf("signin config: %+v", c)
	}

	if w := do(rig.mux, "GET", "/setup/signin/start", nil, false); w.Code != http.StatusForbidden {
		t.Fatalf("start without the setup cookie: %d, want 403", w.Code)
	}
	if w := do(rig.mux, "GET", "/setup/signin/start", nil, true); w.Code != http.StatusFound || rig.flow.logins != 1 {
		t.Fatalf("start: %d, logins %d", w.Code, rig.flow.logins)
	}

	w = do(rig.mux, "GET", "/callback?code=c&state=s", nil, false)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `content="0;url=/setup"`) {
		t.Fatalf("callback: %d\n%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "sub-1") || strings.Contains(w.Body.String(), "sextant.example.org") {
		t.Fatal("the callback page, which carries no setup cookie, shows setup's answers")
	}

	w = do(rig.mux, "GET", "/setup", nil, true)
	for _, want := range []string{"sub-1", "Setup Person", "admins", `value="finish"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Fatalf("result page lacks %q:\n%s", want, w.Body.String())
		}
	}

	w = do(rig.mux, "POST", "/setup/signin", url.Values{"action": {"finish"}}, true)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `content="3;url=/login"`) {
		t.Fatalf("finish: %d\n%s", w.Code, w.Body.String())
	}
	if rig.checks != 1 || rig.restarts != 1 {
		t.Fatalf("checks %d, restarts %d, want 1 and 1", rig.checks, rig.restarts)
	}
	gone := false
	for _, ck := range w.Result().Cookies() {
		gone = gone || (ck.Name == setupCookie && ck.MaxAge < 0)
	}
	if !gone {
		t.Fatal("the setup cookie outlives setup")
	}
	s, k := readDir(t, rig.dir)
	if s.PublicURL != "https://sextant.example.org" || s.OIDCIssuer != "https://id.example.com" || s.OIDCClientID != "sextant" ||
		!slices.Equal(s.OwnerSubjects, []string{"sub-earlier", "sub-1"}) || s.RepoDir != "/data/overlay" {
		t.Fatalf("settings: %+v", s)
	}
	if k.OIDCClientSecret != "s3cret" || k.SessionKey != testSessionKey || k.SecretKey != "secret-key" {
		t.Fatalf("keys: %+v", k)
	}
}

// A console that would refuse to start is worse than one still in setup:
// the old answers go back and the page says what the start would say.
func TestSigninFinishPutsTheOldAnswersBack(t *testing.T) {
	rig := newSigninRig(t, nil)
	rig.checkErr = errors.New("session auth on non-loopback --addr refused")
	beforeS, beforeK := readDir(t, rig.dir)
	do(rig.mux, "POST", "/setup/signin", signinForm("https://sextant.example.org", "https://id.example.com", "sextant", "s3cret"), true)
	do(rig.mux, "GET", "/callback", nil, false)

	w := do(rig.mux, "POST", "/setup/signin", url.Values{"action": {"finish"}}, true)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "non-loopback --addr refused") ||
		!strings.Contains(w.Body.String(), text("setup.signin.would_not_start")) {
		t.Fatalf("finish: %d\n%s", w.Code, w.Body.String())
	}
	if rig.restarts != 0 {
		t.Fatal("restarted into a configuration that would not start")
	}
	s, k := readDir(t, rig.dir)
	if s.OIDCIssuer != "" || k.OIDCClientSecret != "" || !slices.Equal(s.OwnerSubjects, beforeS.OwnerSubjects) || k != beforeK {
		t.Fatalf("answers not put back: %+v %+v", s, k)
	}
}

// Every refusal says what to change, and none reaches the provider.
func TestSigninRefusals(t *testing.T) {
	cases := []struct {
		name string
		form url.Values
		key  string
	}{
		{"plain http elsewhere", signinForm("http://sextant.example.org", "https://id.example.com", "c", "s"), "setup.signin.address_https"},
		{"address with a path", signinForm("https://sextant.example.org/console", "https://id.example.com", "c", "s"), "setup.signin.address_invalid"},
		{"address with credentials", signinForm("https://a:b@sextant.example.org", "https://id.example.com", "c", "s"), "setup.signin.address_invalid"},
		{"no secret", signinForm("https://sextant.example.org", "https://id.example.com", "c", ""), "setup.signin.missing"},
		{"plain http issuer", signinForm("https://sextant.example.org", "http://id.example.com", "c", "s"), "setup.signin.issuer_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newSigninRig(t, nil)
			w := do(rig.mux, "POST", "/setup/signin", tc.form, true)
			if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), text(tc.key)) {
				t.Fatalf("%d, want 422 with %q:\n%s", w.Code, tc.key, w.Body.String())
			}
			if len(rig.got) != 0 {
				t.Fatal("asked the provider anyway")
			}
		})
	}

	rig := newSigninRig(t, nil)
	w := do(rig.mux, "POST", "/setup/signin", signinForm("https://sextant.example.org", "https://down.example.com", "c", "s"), true)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "connection refused") ||
		!strings.Contains(w.Body.String(), text("setup.signin.discovery_failed")) {
		t.Fatalf("discovery failure: %d\n%s", w.Code, w.Body.String())
	}
}

// Plain http on this machine is fine, and the cookies then go without
// the Secure flag, which a browser would not send back over http.
func TestSigninOnLocalhost(t *testing.T) {
	rig := newSigninRig(t, nil)
	w := do(rig.mux, "POST", "/setup/signin", signinForm("http://localhost:8080", "http://127.0.0.1:5556/dex", "c", "s"), true)
	if w.Code != http.StatusOK || rig.got[0].Secure || rig.got[0].RedirectURL != "http://localhost:8080/callback" {
		t.Fatalf("%d %+v", w.Code, rig.got)
	}
}

// Finish needs a test sign-in that came back, from the answers on the
// page now: a new check drops the earlier try, also when it fails.
func TestSigninFinishNeedsACompletedTest(t *testing.T) {
	rig := newSigninRig(t, nil)
	finish := func() {
		t.Helper()
		w := do(rig.mux, "POST", "/setup/signin", url.Values{"action": {"finish"}}, true)
		if w.Code != http.StatusSeeOther || rig.restarts != 0 {
			t.Fatalf("finish without a completed test: %d, restarts %d", w.Code, rig.restarts)
		}
		if s, _ := readDir(t, rig.dir); s.OIDCIssuer != "" {
			t.Fatal("finish wrote answers without a completed test")
		}
	}
	finish()
	do(rig.mux, "POST", "/setup/signin", signinForm("https://sextant.example.org", "https://id.example.com", "c", "s"), true)
	finish()
	do(rig.mux, "GET", "/callback", nil, false)
	do(rig.mux, "POST", "/setup/signin", signinForm("https://sextant.example.org", "https://down.example.com", "c", "s"), true)
	finish()
}

// A callback nobody started, or one the provider turned down, shows the
// reason and a way back, and records nobody.
func TestSigninCallbackFailures(t *testing.T) {
	rig := newSigninRig(t, nil)
	if w := do(rig.mux, "GET", "/callback", nil, false); w.Code != http.StatusBadRequest {
		t.Fatalf("callback with no test started: %d", w.Code)
	}
	do(rig.mux, "POST", "/setup/signin", signinForm("https://sextant.example.org", "https://id.example.com", "c", "s"), true)
	rig.flow.err = errors.New("bad state")
	w := do(rig.mux, "GET", "/callback", nil, false)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "bad state") || strings.Contains(w.Body.String(), "url=/setup") {
		t.Fatalf("failed callback: %d\n%s", w.Code, w.Body.String())
	}
	if w := do(rig.mux, "GET", "/setup", nil, true); strings.Contains(w.Body.String(), `value="finish"`) {
		t.Fatal("a failed test sign-in can be finished")
	}
}

// An address the deployment set is shown, used for the redirect URL and
// never written by setup.
func TestSigninAddressFromTheDeployment(t *testing.T) {
	rig := newSigninRig(t, func(o *SetupOptions) {
		o.Deployment = map[string]bool{"PUBLIC_URL": true}
		o.Settings.PublicURL = "https://fixed.example.org"
	})
	w := do(rig.mux, "GET", "/setup", nil, true)
	if strings.Contains(w.Body.String(), `name="public_url"`) || !strings.Contains(w.Body.String(), "https://fixed.example.org/callback") {
		t.Fatalf("fixed address not shown as fixed:\n%s", w.Body.String())
	}
	do(rig.mux, "POST", "/setup/signin", signinForm("https://other.example.org", "https://id.example.com", "c", "s"), true)
	if rig.got[0].RedirectURL != "https://fixed.example.org/callback" {
		t.Fatalf("redirect = %q", rig.got[0].RedirectURL)
	}
	do(rig.mux, "GET", "/callback", nil, false)
	do(rig.mux, "POST", "/setup/signin", url.Values{"action": {"finish"}}, true)
	if s, _ := readDir(t, rig.dir); s.PublicURL != "" || s.OIDCIssuer != "https://id.example.com" {
		t.Fatalf("settings: %+v", s)
	}
}

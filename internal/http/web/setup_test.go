package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/config"
)

const testToken = "the-setup-token"

func newTestSetup(t *testing.T, opt SetupOptions) *http.ServeMux {
	t.Helper()
	if opt.DataDir == "" {
		opt.DataDir = t.TempDir()
	}
	opt.Token = testToken
	s, err := NewSetup(opt)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	s.Routes(mux)
	return mux
}

func do(mux http.Handler, method, target string, form url.Values, cookie bool) *httptest.ResponseRecorder {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(method, target, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookie {
		r.AddCookie(&http.Cookie{Name: setupCookie, Value: testToken})
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	return w
}

// Only the holder of the link in the log gets in, and the token leaves the
// address bar on the first request.
func TestSetupNeedsTheToken(t *testing.T) {
	mux := newTestSetup(t, SetupOptions{})

	for _, target := range []string{"/setup", "/setup?token=wrong", "/setup/recovery-key"} {
		if w := do(mux, "GET", target, nil, false); w.Code != http.StatusForbidden {
			t.Errorf("%s without the token: %d, want 403", target, w.Code)
		}
	}
	w := do(mux, "GET", "/setup?token="+testToken, nil, false)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/setup" {
		t.Fatalf("right token: %d to %q, want a redirect to /setup", w.Code, w.Header().Get("Location"))
	}
	c := w.Result().Cookies()
	if len(c) != 1 || c[0].Value != testToken || !c[0].HttpOnly || c[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie: %+v", c)
	}
	if w := do(mux, "GET", "/setup", nil, true); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "/setup/keys") {
		t.Fatalf("with the cookie: %d, keys step not shown", w.Code)
	}
}

// Everyone else, on the pages a running console would answer, is told
// where the link is and is not given anything else.
func TestSetupTellsOthersWhereTheLinkIs(t *testing.T) {
	mux := newTestSetup(t, SetupOptions{})
	for _, target := range []string{"/", "/login"} {
		w := do(mux, "GET", target, nil, false)
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "setup:") {
			t.Errorf("%s: %d %q", target, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), testToken) {
			t.Errorf("%s leaks the token", target)
		}
	}
}

// The keys reach the disk only once the operator says the recovery file is
// stored, and the file they downloaded holds the key that was written.
func TestKeysAreWrittenAfterTheRecoveryFileIsStored(t *testing.T) {
	dir := t.TempDir()
	mux := newTestSetup(t, SetupOptions{DataDir: dir})

	if w := do(mux, "POST", "/setup/keys", url.Values{}, true); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("without the confirmation: %d, want 422", w.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, config.KeysFile)); !os.IsNotExist(err) {
		t.Fatalf("keys written before the recovery file was confirmed (%v)", err)
	}

	dl := do(mux, "GET", "/setup/recovery-key", nil, true)
	if dl.Code != http.StatusOK || dl.Header().Get("Cache-Control") != "no-store" ||
		!strings.Contains(dl.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("recovery download: %d %v", dl.Code, dl.Header())
	}

	if w := do(mux, "POST", "/setup/keys", url.Values{"stored": {"yes"}}, true); w.Code != http.StatusSeeOther {
		t.Fatalf("with the confirmation: %d, want 303", w.Code)
	}
	_, k, err := config.ReadDataDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(dl.Body.String(), "SEXTANT_SECRET_KEY="+k.SecretKey+"\n") {
		t.Fatalf("the downloaded key is not the one written")
	}
	// What setup wrote, a normal start accepts.
	cfg, err := config.Load([]string{"--data-dir", dir}, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SessionKey) != 32 || cfg.CheckinToken == "" || cfg.SecretKey == "" {
		t.Fatalf("written keys do not load: %d %q", len(cfg.SessionKey), cfg.CheckinToken)
	}
	if w := do(mux, "GET", "/setup", nil, true); !strings.Contains(w.Body.String(), `aria-current="step"`) ||
		strings.Contains(w.Body.String(), "/setup/keys") {
		t.Fatalf("still on the keys step after writing them")
	}
}

// A secret the deployment set is never copied to the disk and never
// offered for download.
func TestKeysNeverCopyTheDeploymentsSecret(t *testing.T) {
	dir := t.TempDir()
	mux := newTestSetup(t, SetupOptions{DataDir: dir,
		Keys:       config.Keys{SecretKey: "from-the-environment"},
		Deployment: map[string]bool{"SECRET_KEY": true}})

	if w := do(mux, "GET", "/setup/recovery-key", nil, true); w.Code != http.StatusNotFound {
		t.Fatalf("recovery download of a deployment secret: %d, want 404", w.Code)
	}
	if w := do(mux, "POST", "/setup/keys", url.Values{}, true); w.Code != http.StatusSeeOther {
		t.Fatalf("no confirmation is needed for a key setup did not make: %d", w.Code)
	}
	raw, err := os.ReadFile(filepath.Join(dir, config.KeysFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "from-the-environment") || strings.Contains(string(raw), "secretKey") {
		t.Fatalf("deployment secret copied to disk: %s", raw)
	}
}

// A key from an earlier, unfinished setup is kept: replacing it would
// orphan whatever it already sealed.
func TestKeysAlreadyOnDiskAreKept(t *testing.T) {
	dir := t.TempDir()
	if err := config.WriteKeys(dir, config.Keys{CheckinToken: "earlier"}); err != nil {
		t.Fatal(err)
	}
	mux := newTestSetup(t, SetupOptions{DataDir: dir, Keys: config.Keys{CheckinToken: "earlier"}})
	if w := do(mux, "POST", "/setup/keys", url.Values{"stored": {"yes"}}, true); w.Code != http.StatusSeeOther {
		t.Fatalf("%d", w.Code)
	}
	_, k, _ := config.ReadDataDir(dir)
	if k.CheckinToken != "earlier" || k.SessionKey == "" || k.SecretKey == "" {
		t.Fatalf("got %+v", k)
	}
}

func TestSetupRefusesCrossOriginPost(t *testing.T) {
	dir := t.TempDir()
	mux := newTestSetup(t, SetupOptions{DataDir: dir})
	r := httptest.NewRequest("POST", "/setup/keys", strings.NewReader("stored=yes"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://evil.example.com")
	r.AddCookie(&http.Cookie{Name: setupCookie, Value: testToken})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin POST: %d, want 403", w.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, config.KeysFile)); !os.IsNotExist(err) {
		t.Fatal("a cross-origin POST wrote the keys")
	}
}

func TestNewSetupRefusesWithoutTokenOrDir(t *testing.T) {
	if _, err := NewSetup(SetupOptions{DataDir: t.TempDir()}); err == nil {
		t.Error("no token accepted")
	}
	if _, err := NewSetup(SetupOptions{Token: "x"}); err == nil {
		t.Error("no data directory accepted")
	}
}

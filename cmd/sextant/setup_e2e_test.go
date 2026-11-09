package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

// testIDP is an identity provider that signs in one person without asking:
// discovery, an authorize endpoint that sends the browser straight back
// with a code, a token endpoint and its key.
type testIDP struct {
	srv *httptest.Server
	key *rsa.PrivateKey

	mu     sync.Mutex
	nonces map[string]string
}

func newTestIDP(t *testing.T) *testIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &testIDP{key: key, nonces: map[string]string{}}
	mux := http.NewServeMux()
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	iss := idp.srv.URL
	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"issuer": iss, "authorization_endpoint": iss + "/authorize",
			"token_endpoint": iss + "/token", "jwks_uri": iss + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"}})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		b := make([]byte, 8)
		_, _ = rand.Read(b)
		code := hex.EncodeToString(b)
		idp.mu.Lock()
		idp.nonces[code] = q.Get("nonce")
		idp.mu.Unlock()
		http.Redirect(w, r, q.Get("redirect_uri")+"?code="+code+"&state="+url.QueryEscape(q.Get("state")), http.StatusFound)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		idp.mu.Lock()
		nonce := idp.nonces[r.PostFormValue("code")]
		idp.mu.Unlock()
		now := time.Now()
		payload, _ := json.Marshal(map[string]any{"iss": iss, "aud": "sextant", "sub": "sub-e2e",
			"name": "Setup Person", "nonce": nonce, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key},
			(&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), "k1"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jws, err := signer.Sign(payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		idt, _ := jws.CompactSerialize()
		writeJSON(w, map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": idt})
	})
	return idp
}

// syncBuffer is a log both runs write to while the test reads it.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func eventually(t *testing.T, what string, f func() bool) {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if f() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// From an empty data directory to a signed-in owner, through the pages and
// the restart, with nothing set but the listen address: the first-start
// promise of design 0013, short of the database and the gate, which come
// with the install path.
func TestSetupEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("needs git to start the fleet repository")
	}
	idp := newTestIDP(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	base := "http://" + addr
	args := []string{"--addr", addr, "--data-dir", t.TempDir()}
	logs := &syncBuffer{}
	start := func() chan error {
		ctx, cancel := context.WithCancel(context.Background())
		done, exited := make(chan error, 1), make(chan struct{})
		go func() { done <- run(ctx, args, noEnv, logs); close(exited) }()
		t.Cleanup(func() {
			cancel()
			select {
			case <-exited:
			case <-time.After(20 * time.Second):
				t.Error("the console did not stop")
			}
		})
		return done
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 30 * time.Second}
	send := func(method, path string, form url.Values) (*http.Response, string) {
		t.Helper()
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequest(method, base+path, body)
		if err != nil {
			t.Fatal(err)
		}
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	expect := func(resp *http.Response, page, want string) {
		t.Helper()
		if resp.StatusCode != http.StatusOK || !strings.Contains(page, want) {
			t.Fatalf("%s: %d, want 200 with %q\n%s\nlog:\n%s", resp.Request.URL, resp.StatusCode, want, page, logs.String())
		}
	}

	first := start()
	var token string
	eventually(t, "the setup link in the log", func() bool {
		s := logs.String()
		i := strings.Index(s, "/setup?token=")
		if i < 0 {
			return false
		}
		token = strings.FieldsFunc(s[i+len("/setup?token="):], func(r rune) bool { return r == '"' || r == ' ' || r == '\n' })[0]
		return true
	})

	resp, page := send("GET", "/setup?token="+token, nil)
	expect(resp, page, `action="/setup/keys"`)
	resp, page = send("POST", "/setup/keys", url.Values{"stored": {"yes"}})
	expect(resp, page, `action="/setup/repo"`)
	resp, page = send("POST", "/setup/repo", url.Values{"choice": {"new"}})
	expect(resp, page, `name="public_url"`)
	resp, page = send("POST", "/setup/signin", url.Values{"action": {"check"}, "public_url": {base},
		"issuer": {idp.srv.URL}, "client_id": {"sextant"}, "client_secret": {"s3cret"}})
	expect(resp, page, `href="/setup/signin/start"`)
	resp, page = send("GET", "/setup/signin/start", nil)
	expect(resp, page, `content="0;url=/setup"`)
	resp, page = send("GET", "/setup", nil)
	expect(resp, page, "sub-e2e")
	resp, page = send("POST", "/setup/signin", url.Values{"action": {"finish"}})
	expect(resp, page, `content="3;url=/login"`)

	select {
	case err := <-first:
		if !errors.Is(err, errRestart) {
			t.Fatalf("setup ended the run with %v, want a restart", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("setup finished but the console did not restart")
	}

	start()
	eventually(t, "the console after the restart", func() bool {
		r, err := http.Get(base + "/login")
		if err != nil {
			return false
		}
		_ = r.Body.Close()
		return r.StatusCode == http.StatusOK
	})
	resp, page = send("GET", "/login/start", nil)
	if resp.StatusCode != http.StatusOK || resp.Request.URL.Path != "/" {
		t.Fatalf("sign-in after setup ended at %s with %d\n%s\nlog:\n%s", resp.Request.URL, resp.StatusCode, page, logs.String())
	}
	if !strings.Contains(page, "Setup Person") {
		t.Fatalf("the console does not show the person who set it up:\n%s", page)
	}
	if resp, page = send("GET", "/api/v1/devices", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("the API turns the owner away: %d\n%s", resp.StatusCode, page)
	}
}

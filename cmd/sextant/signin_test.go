package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/adapters/git"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/starter"
)

// An identity provider that comes up after the console - a cluster that
// restarts, a provider being upgraded - used to leave the console without a
// login until somebody restarted it. Now the console says it is waiting,
// asks again, and signs people in once the provider answers.
func TestSignInWaitsForTheIdentityProvider(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("needs git for the fleet repository")
	}
	old := signinRetry
	signinRetry = 50 * time.Millisecond
	t.Cleanup(func() { signinRetry = old })

	var up atomic.Bool
	var issuer string
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up.Load() {
			http.Error(w, "starting", http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer,
			"authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token",
			"jwks_uri": issuer + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"}})
	}))
	t.Cleanup(idp.Close)
	issuer = idp.URL

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	base := "http://" + addr
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	env := func(k string) string {
		return map[string]string{"SEXTANT_OIDC_CLIENT_SECRET": "test-secret", "SEXTANT_SESSION_KEY": key}[k]
	}
	repo := filepath.Join(t.TempDir(), "fleet")
	if err := git.Seed(context.Background(), repo, starter.Overlay(), "Start the fleet repository"); err != nil {
		t.Fatal(err)
	}
	args := []string{"--addr", addr, "--data-dir", t.TempDir(), "--repo", repo, "--oidc-issuer", issuer,
		"--oidc-client-id", "sextant", "--public-url", base}
	logs := &syncBuffer{}
	ctx, cancel := context.WithCancel(context.Background())
	exited := make(chan struct{})
	go func() { _ = run(ctx, args, env, logs); close(exited) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-exited:
		case <-time.After(20 * time.Second):
			t.Error("the console did not stop")
		}
	})

	client := &http.Client{Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(path string) (int, string, string) {
		resp, err := client.Get(base + path)
		if err != nil {
			return 0, "", ""
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, resp.Header.Get("Location"), string(b)
	}

	// The provider is down: the console is there, and says so in words.
	eventually(t, "the console to answer", func() bool { st, _, _ := get("/login"); return st == http.StatusOK })
	st, _, body := get("/login/start")
	if st != http.StatusServiceUnavailable || !strings.Contains(body, "has not answered") {
		t.Fatalf("sign-in while the provider is down: %d %q, want 503 in words\nlog:\n%s", st, body, logs.String())
	}
	if st, _, _ := get("/readyz"); st == http.StatusOK {
		t.Fatal("ready without a way to sign in")
	}

	// The provider comes up: sign-in follows without a restart.
	up.Store(true)
	eventually(t, "sign-in to send people to the provider", func() bool {
		st, loc, _ := get("/login/start")
		return st == http.StatusFound && strings.HasPrefix(loc, issuer+"/authorize")
	})
	if st, _, body := get("/readyz"); st != http.StatusOK {
		t.Fatalf("not ready once the provider answers: %d %s\nlog:\n%s", st, body, logs.String())
	}
}

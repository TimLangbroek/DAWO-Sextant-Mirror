package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/forge"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/config"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/logging"
)

func TestSetupModeOnlyWhenNobodyCanSignIn(t *testing.T) {
	cases := []struct {
		cfg  config.Config
		want bool
	}{
		{config.Config{}, true},
		{config.Config{OIDCIssuer: "https://id.example.com"}, false},
		{config.Config{DevAuth: true}, false},
	}
	for _, c := range cases {
		if got := setupMode(&c.cfg); got != c.want {
			t.Errorf("%+v: setupMode = %v, want %v", c.cfg, got, c.want)
		}
	}
}

func TestSetupLink(t *testing.T) {
	cases := []struct {
		cfg  config.Config
		want string
	}{
		{config.Config{PublicURL: "https://console.example.com/", Addr: "0.0.0.0:8080"}, "https://console.example.com/setup?token=T"},
		{config.Config{Addr: "0.0.0.0:8080"}, "http://localhost:8080/setup?token=T"},
		{config.Config{Addr: ":8080"}, "http://localhost:8080/setup?token=T"},
		{config.Config{Addr: "127.0.0.1:9000"}, "http://127.0.0.1:9000/setup?token=T"},
		{config.Config{Addr: "[::1]:8080"}, "http://[::1]:8080/setup?token=T"},
	}
	for _, c := range cases {
		if got := setupLink(&c.cfg, "T"); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.cfg, got, c.want)
		}
	}
}

// The link in the log opens setup; that line is the only way in.
func TestMountSetupLogsAWorkingLink(t *testing.T) {
	var buf bytes.Buffer
	cfg := &config.Config{Addr: "127.0.0.1:8080", DataDir: t.TempDir()}
	mux := http.NewServeMux()
	if !mountSetup(mux, cfg, logging.New(&buf, "text", "info")) {
		t.Fatal("setup not mounted")
	}
	line := buf.String()
	i := strings.Index(line, "/setup?token=")
	if i < 0 {
		t.Fatalf("no link logged: %s", line)
	}
	tok := strings.Fields(line[i+len("/setup?token="):])[0]
	tok = strings.TrimRight(tok, `"`)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/setup?token="+tok, nil))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("logged link answers %d, want the redirect that sets the cookie", w.Code)
	}

	// Without a data directory there is nowhere to keep answers, and the
	// caller falls back to the identity line instead of a setup that
	// cannot save.
	if mountSetup(http.NewServeMux(), &config.Config{Addr: "127.0.0.1:8080"}, logging.New(io.Discard, "text", "info")) {
		t.Fatal("setup mounted without a data directory")
	}
}

type fakeForge struct {
	enabled, stored bool
	setErr          error
	set             []string
}

func (f *fakeForge) Enabled() bool { return f.enabled }
func (f *fakeForge) Current(context.Context) (forge.Identity, bool, error) {
	return forge.Identity{}, f.stored, nil
}
func (f *fakeForge) Set(_ context.Context, host, user, token, by string) error {
	f.set = append(f.set, host+"|"+user+"|"+token+"|"+by)
	return f.setErr
}

func TestHandOverSetupForge(t *testing.T) {
	cases := []struct {
		name        string
		f           fakeForge
		wantSet     bool
		wantDropped bool
	}{
		{"moves into the store", fakeForge{enabled: true}, true, true},
		{"a stored credential wins", fakeForge{enabled: true, stored: true}, false, true},
		{"no sealing key keeps it", fakeForge{}, false, false},
		{"a failed store keeps it", fakeForge{enabled: true, setErr: errors.New("db down")}, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := config.WriteKeys(dir, config.Keys{ForgeToken: "tok", CheckinToken: "ct"}); err != nil {
				t.Fatal(err)
			}
			cfg := &config.Config{DataDir: dir, SetupForge: &config.SetupForge{Host: "git.example.com", User: "alice", Token: "tok"}}
			f := c.f
			handOverSetupForge(context.Background(), &f, cfg, logging.New(io.Discard, "text", "info"))
			if got := len(f.set) == 1; got != c.wantSet {
				t.Fatalf("Set called: %v, want %v", f.set, c.wantSet)
			}
			if c.wantSet && f.set[0] != "git.example.com|alice|tok|setup" {
				t.Fatalf("Set(%s)", f.set[0])
			}
			_, k, _ := config.ReadDataDir(dir)
			if dropped := k.ForgeToken == ""; dropped != c.wantDropped {
				t.Fatalf("token dropped: %v, want %v", dropped, c.wantDropped)
			}
			if k.CheckinToken != "ct" {
				t.Fatal("other keys lost")
			}
			if (cfg.SetupForge == nil) != c.wantDropped {
				t.Fatal("cfg.SetupForge not in step with keys.json")
			}
		})
	}
	// Nothing to hand over touches nothing.
	f := &fakeForge{enabled: true}
	handOverSetupForge(context.Background(), f, &config.Config{}, logging.New(io.Discard, "text", "info"))
	if len(f.set) != 0 {
		t.Fatal("Set called without a setup credential")
	}
}

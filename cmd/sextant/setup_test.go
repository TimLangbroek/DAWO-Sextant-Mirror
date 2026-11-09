package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

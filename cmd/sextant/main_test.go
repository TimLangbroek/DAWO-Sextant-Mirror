package main

// Smoke coverage for the wiring in main.go/capabilities.go that does not
// require Postgres, git or nix: the health/metrics-only capability path and
// the fail-fast config path. Anything past a valid config starts a real
// listener and background workers (server.Run blocks until ctx/signal), so
// it is out of scope for a unit-level smoke test.

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/config"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/health"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/logging"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/metrics"
)

// noEnv is a config.Getenv with nothing set, the same injectable seam
// config_test.go uses - no process environment is touched.
func noEnv(string) string { return "" }

// TestBuildCapabilitiesNoRepo covers the deployment shape used for probes
// and smoke tests: no --repo means no config plane, so buildCapabilities
// must return cleanly with nothing to mount and nothing to release.
func TestBuildCapabilitiesNoRepo(t *testing.T) {
	cfg, err := config.Load(nil, noEnv)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.RepoDir != "" {
		t.Fatalf("precondition: RepoDir = %q, want empty", cfg.RepoDir)
	}

	log := logging.New(io.Discard, "text", "info")
	checks := health.New(time.Second)

	caps, cleanup, err := buildCapabilities(context.Background(), cfg, log, checks, metrics.New())
	if err != nil {
		t.Fatalf("buildCapabilities: %v", err)
	}
	if caps != nil {
		t.Errorf("caps = %v, want nil", caps)
	}
	if cleanup != nil {
		t.Errorf("cleanup = %v, want nil", cleanup)
	}
}

// TestRunFailsFastOnInvalidConfig exercises run's flag-parsing/config wiring:
// a config that fails validation must surface a clear error before any
// server, repo or background worker is touched.
func TestRunFailsFastOnInvalidConfig(t *testing.T) {
	err := run(context.Background(), []string{"--log-level", "deafening"}, noEnv, io.Discard)
	if err == nil {
		t.Fatal("run: want error for invalid --log-level, got nil")
	}
	if !strings.Contains(err.Error(), "log-level") {
		t.Errorf("run error = %q, want it to mention log-level", err)
	}
}

// TestRunFailsFastOnWriteWithoutRepo covers a second, distinct validation
// branch reachable purely through flag parsing, so the fail-fast path isn't
// only exercised via the log-level case above.
func TestRunFailsFastOnWriteWithoutRepo(t *testing.T) {
	err := run(context.Background(), []string{"--write"}, noEnv, io.Discard)
	if err == nil {
		t.Fatal("run: want error for --write without --repo, got nil")
	}
	if !strings.Contains(err.Error(), "--repo") {
		t.Errorf("run error = %q, want it to mention --repo", err)
	}
}

// check-config says what a start would run with, or why it would not
// start, and never prints a secret.
func TestCheckConfig(t *testing.T) {
	var out, errOut strings.Builder
	getenv := func(k string) string {
		return map[string]string{"SEXTANT_PG_DSN": "postgres://u:hunter2@db/sextant"}[k]
	}
	if code := checkConfig([]string{"--addr", "127.0.0.1:9000", "--data-dir", t.TempDir()}, getenv, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	for _, want := range []string{"will start", "127.0.0.1:9000", "setup mode", "database:         set"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "hunter2") {
		t.Fatal("check-config printed the database password")
	}

	out.Reset()
	if code := checkConfig([]string{"--write", "--gate", "none"}, noEnv, &out, &errOut); code != 1 {
		t.Fatalf("exit %d for a refused configuration", code)
	}
	if !strings.Contains(errOut.String(), "nothing checks them") || !strings.Contains(errOut.String(), "fix:") || out.Len() != 0 {
		t.Fatalf("refusal not reported: stdout %q stderr %q", out.String(), errOut.String())
	}
}

package web_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/adapters/git"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/web"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// Every page says when the console stopped following its repository, and
// why when the reason is its own: a stale fleet must not read as the
// current one. A fetch failure shows no git output, which can carry the
// remote's address and a credential in it.
func TestEveryPageSaysTheConsoleStoppedFollowing(t *testing.T) {
	dir, bare, other := t.TempDir(), filepath.Join(t.TempDir(), "remote.git"), t.TempDir()
	run := func(in string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", in}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "SSH_ASKPASS=", "GIT_ASKPASS=")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(in, doc string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(in, "fleet.json"), []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(dir, "init", "-q", "-b", "main")
	write(dir, seedFleet)
	if err := os.WriteFile(filepath.Join(dir, "catalog.json"), []byte(seedCatalog), 0o644); err != nil {
		t.Fatal(err)
	}
	run(dir, "add", ".")
	run(dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "seed")
	run(dir, "clone", "-q", "--bare", dir, bare)
	run(dir, "remote", "add", "origin", bare)
	run(dir, "fetch", "-q", "origin")
	run(other, "clone", "-q", bare, other)

	repo, err := git.Open(dir, "origin")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := app.NewConfigService(repo,
		ports.GateFunc(func(context.Context, string, []string) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	srv, err := web.New(web.Services{Config: cfg}, web.DevSessions{}, true,
		nil, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	srv.Routes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		cfg.SyncLoop(ctx, 20*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(done)
	}()
	defer func() { cancel(); <-done }()

	page := func() string {
		t.Helper()
		resp, err := client().Get(ts.URL + "/policies")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if ok() {
				return
			}
		}
		t.Fatalf("never: %s", what)
	}

	if p := page(); strings.Contains(p, "data-sync-fault") {
		t.Fatal("a console that follows its repository says it does not")
	}

	write(other, strings.Replace(seedFleet, `"version": 3,`, `"version": 3, "polices": {},`, 1))
	run(other, "-c", "user.name=e", "-c", "user.email=e@e", "commit", "-q", "-am", "a typo")
	run(other, "push", "-q", "origin", "main")
	waitFor("the read fault", func() bool { return cfg.SyncFault() != nil })
	p := page()
	for _, want := range []string{"data-sync-fault", "cannot read the latest state", "&#34;polices&#34;", " UTC."} {
		if !strings.Contains(p, want) {
			t.Errorf("a console that cannot read its repository does not show %q", want)
		}
	}

	if err := os.RemoveAll(bare); err != nil {
		t.Fatal(err)
	}
	waitFor("the fetch fault", func() bool { f := cfg.SyncFault(); return f != nil && !f.Read })
	p = page()
	if !strings.Contains(p, "cannot reach its fleet repository") {
		t.Error("a console that cannot reach its repository does not say so")
	}
	if strings.Contains(p, bare) || strings.Contains(p, "fatal:") {
		t.Error("the page shows git's own message, which can carry the remote's address")
	}
}

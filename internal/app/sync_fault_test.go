package app

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/adapters/git"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// A console that cannot follow its repository keeps serving the last copy
// it could read, which is right: a flaky remote must not take it down. But
// the reason went only to the log, so an operator read a stale fleet as the
// current one. SyncFault says why and since when, and goes away by itself
// once the repository can be read again.
func TestSyncFaultSaysWhyTheConsoleStoppedFollowing(t *testing.T) {
	_, dir := newService(t, nil)
	bare := filepath.Join(t.TempDir(), "remote.git")
	sh(t, dir, "clone", "-q", "--bare", dir, bare)
	sh(t, dir, "remote", "add", "origin", bare)
	sh(t, dir, "push", "-q", "origin", "main")
	repo, err := git.Open(dir, "origin")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewConfigService(repo, ports.GateFunc(func(context.Context, string, []string) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if f := svc.SyncFault(); f != nil {
		t.Fatalf("a fresh console reports %+v", f)
	}

	other := t.TempDir()
	sh(t, other, "clone", "-q", bare, other)
	push := func(doc, msg string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(other, "fleet.json"), []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		sh(t, other, "add", "fleet.json")
		sh(t, other, "-c", "user.name=e", "-c", "user.email=e@e", "commit", "-q", "-m", msg)
		sh(t, other, "push", "-q", "origin", "main")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.SyncLoop(ctx, 20*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
		close(done)
	}()
	defer func() { cancel(); <-done }()
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if ok() {
				return
			}
		}
		t.Fatalf("never: %s (fault %+v)", what, svc.SyncFault())
	}

	// Somebody pushes a document this build refuses.
	push(strings.Replace(seedFleet, `"version": 3,`, `"version": 3, "polices": {},`, 1), "a typo")
	waitFor("a read fault naming the field", func() bool {
		f := svc.SyncFault()
		return f != nil && f.Read && strings.Contains(f.Detail, `"polices"`)
	})
	since := svc.SyncFault().Since
	time.Sleep(60 * time.Millisecond) // a few more failing ticks
	if f := svc.SyncFault(); f == nil || !f.Since.Equal(since) {
		t.Fatalf("the fault's start moved with every tick: %v, then %+v", since, f)
	}
	if svc.Fleet().Org.Settings["desktop"] != "plasma" {
		t.Fatal("the snapshot changed although the document could not be read")
	}

	// Corrected: the fault clears and the console follows again.
	push(strings.Replace(seedFleet, `"desktop": "plasma"`, `"desktop": "gnome"`, 1), "fixed")
	waitFor("the fault clears", func() bool {
		return svc.SyncFault() == nil && svc.Fleet().Org.Settings["desktop"] == "gnome"
	})

	// The remote goes away: a fault without detail, since git's own message
	// can carry the remote's address.
	if err := os.RemoveAll(bare); err != nil {
		t.Fatal(err)
	}
	waitFor("a fetch fault", func() bool {
		f := svc.SyncFault()
		return f != nil && !f.Read && f.Detail == ""
	})
}

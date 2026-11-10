package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
)

func TestPrepareRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("needs git") // the nix build sandbox has none; `just test` runs it
	}
	ctx := context.Background()
	remote := filepath.Join(t.TempDir(), "fleet.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "main", remote).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	dir := filepath.Join(t.TempDir(), "overlay")
	var out, errOut strings.Builder
	if code := prepareRepo(ctx, []string{"--repo", dir, "--url", "file://" + remote, "--public-url", "https://console.example.org/"}, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "starter fleet") {
		t.Fatalf("stdout %q", out.String())
	}
	// What was pushed is the starter every other path seeds, with this
	// console in its console block: that is how devices find it.
	if _, err := os.Stat(filepath.Join(dir, "fleet.json")); err != nil {
		t.Fatal(err)
	}
	pushed, err := exec.Command("git", "--git-dir", remote, "show", "main:fleet.json").Output()
	if err != nil {
		t.Fatal(err)
	}
	f, err := fleet.Decode(pushed)
	if err != nil {
		t.Fatal(err)
	}
	if f.Console == nil || f.Console.URL != "https://console.example.org" || f.Console.Branch != "main" {
		t.Fatalf("pushed console block %+v", f.Console)
	}

	for _, args := range [][]string{{"--repo", dir}, {"--url", "file://" + remote}, {"--nonsense"}} {
		errOut.Reset()
		if code := prepareRepo(ctx, args, &out, &errOut); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
	errOut.Reset()
	if code := prepareRepo(ctx, []string{"--repo", dir, "--url", "file://" + remote, "--branch", "nope"}, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "sextant:") {
		t.Fatalf("exit %d: %q", code, errOut.String())
	}
}

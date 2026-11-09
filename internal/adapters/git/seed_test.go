package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

func commits(t *testing.T, dir string) int {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-list", "--count", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSeedCreatesARepositoryOnce(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "data", "overlay")
	files := fstest.MapFS{"fleet.json": {Data: []byte(`{"version":3}`)}, "hardware/x.nix": {Data: []byte("{}")}}

	if err := Seed(ctx, dir, files, "seed"); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir, ""); err != nil {
		t.Fatalf("seeded dir does not open as a repo: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "hardware", "x.nix")); string(b) != "{}" {
		t.Fatalf("nested file not seeded: %q", b)
	}
	if out, _ := exec.Command("git", "-C", dir, "status", "--porcelain").Output(); len(out) != 0 {
		t.Fatalf("seed left uncommitted files: %s", out)
	}
	if out, _ := exec.Command("git", "-C", dir, "branch", "--show-current").Output(); strings.TrimSpace(string(out)) != "main" {
		t.Fatalf("branch %q, want main", out)
	}

	// A second seed leaves real history alone.
	if err := os.WriteFile(filepath.Join(dir, "fleet.json"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Seed(ctx, dir, files, "seed"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "fleet.json")); string(b) != "edited" {
		t.Fatal("second seed overwrote the repository")
	}
	if commits(t, dir) != 1 {
		t.Fatal("second seed committed")
	}
}

func TestSeedRefusesAForeignDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "precious"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Seed(context.Background(), dir, fstest.MapFS{"a": {Data: []byte("a")}}, "seed"); err == nil {
		t.Fatal("seeded into a directory that is not a repository")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "precious")); string(b) != "x" {
		t.Fatal("foreign directory touched")
	}
}

// A seed that fails halfway leaves nothing the next start would take for
// a repository. An empty file set fails at the commit, after init.
func TestSeedFailureLeavesNothing(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "overlay")
	if err := Seed(context.Background(), dir, fstest.MapFS{}, "seed"); err == nil {
		t.Fatal("empty seed succeeded")
	}
	entries, _ := os.ReadDir(parent)
	if len(entries) != 0 {
		t.Fatalf("failed seed left %v", entries)
	}
}

package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

var starterFiles = fstest.MapFS{"fleet.json": {Data: []byte(`{"version":3}`)}}

// bare makes an empty bare repository and returns its file:// address.
func bare(t *testing.T) (string, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "remote.git")
	gitT(t, "", "init", "-q", "--bare", "-b", "main", dir)
	return dir, "file://" + dir
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// remoteHead is the commit branch points at in the bare repository, or "".
func remoteHead(t *testing.T, bareDir, branch string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", bareDir, "rev-parse", "--verify", "-q", "refs/heads/"+branch).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// A repository just created on a forge is empty. Pointing at it is enough:
// it gets the starter fleet and the remote has it too.
func TestPrepareFillsAnEmptyRemote(t *testing.T) {
	ctx := context.Background()
	bareDir, url := bare(t)
	dir := filepath.Join(t.TempDir(), "data", "overlay")

	said, err := Prepare(ctx, dir, url, "main", starterFiles, "Start the fleet repository")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(said, "was empty") {
		t.Fatalf("said %q", said)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "fleet.json")); string(b) != `{"version":3}` {
		t.Fatalf("fleet.json = %q", b)
	}
	head := remoteHead(t, bareDir, "main")
	if head == "" || head != gitT(t, dir, "rev-parse", "HEAD") {
		t.Fatalf("remote main %q is not the seeded commit", head)
	}

	// A restart changes nothing.
	said, err = Prepare(ctx, dir, url, "main", starterFiles, "again")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(said, "up to date") || commits(t, dir) != 1 || remoteHead(t, bareDir, "main") != head {
		t.Fatalf("second run: %q, %d commits", said, commits(t, dir))
	}

	// On a branch that is not git's default the seed lands on that branch,
	// not on whatever an empty clone happened to point HEAD at.
	bareDir, url = bare(t)
	dir = filepath.Join(t.TempDir(), "overlay")
	if _, err := Prepare(ctx, dir, url, "fleet", starterFiles, "seed"); err != nil {
		t.Fatal(err)
	}
	if remoteHead(t, bareDir, "fleet") == "" || remoteHead(t, bareDir, "main") != "" {
		t.Fatal("seed not on the fleet branch")
	}
}

// An empty directory is what a fresh volume or a tmpfiles rule leaves, and
// it is used as it is. A clone that fails leaves it empty for the next try.
func TestPrepareUsesAnEmptyDirectory(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if _, err := Prepare(ctx, dir, "file:///nonexistent/remote.git", "main", starterFiles, "s"); err == nil {
		t.Fatal("unreachable remote accepted")
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("after a failed clone: %v, %d entries", err, len(entries))
	}
	bareDir, url := bare(t)
	said, err := Prepare(ctx, dir, url, "main", starterFiles, "seed")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(said, "was empty") || remoteHead(t, bareDir, "main") != gitT(t, dir, "rev-parse", "HEAD") {
		t.Fatalf("said %q", said)
	}
}

// A remote with a fleet already in it is cloned and never seeded over.
func TestPrepareClonesAFilledRemote(t *testing.T) {
	ctx := context.Background()
	bareDir, url := bare(t)
	work := filepath.Join(t.TempDir(), "work")
	gitT(t, "", "clone", "-q", url, work)
	if err := os.WriteFile(filepath.Join(work, "fleet.json"), []byte(`{"version":3,"theirs":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, work, "add", "-A")
	gitT(t, work, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-qm", "theirs")
	// The fleet lives on a branch that is not the remote's default, which
	// a plain clone would not check out.
	gitT(t, work, "push", "-q", "origin", "HEAD:main", "HEAD:fleet")
	before := remoteHead(t, bareDir, "fleet")

	dir := filepath.Join(t.TempDir(), "overlay")
	said, err := Prepare(ctx, dir, url, "fleet", starterFiles, "seed")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(said, "cloned") {
		t.Fatalf("said %q", said)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "fleet.json")); !strings.Contains(string(b), "theirs") {
		t.Fatalf("seeded over the remote's fleet: %q", b)
	}
	if br := gitT(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); br != "fleet" {
		t.Fatalf("on branch %q", br)
	}
	if remoteHead(t, bareDir, "fleet") != before {
		t.Fatal("the remote moved")
	}
}

// Someone's repository on another branch is not empty: a seeded branch
// beside their work would split the fleet, so it is refused.
func TestPrepareRefusesARepositoryWithoutTheBranch(t *testing.T) {
	ctx := context.Background()
	bareDir, url := bare(t)
	work := filepath.Join(t.TempDir(), "work")
	gitT(t, "", "clone", "-q", url, work)
	gitT(t, work, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x")
	gitT(t, work, "push", "-q", "origin", "HEAD:trunk")

	dir := filepath.Join(t.TempDir(), "overlay")
	_, err := Prepare(ctx, dir, url, "main", starterFiles, "seed")
	if err == nil || !strings.Contains(err.Error(), `no branch "main"`) {
		t.Fatalf("err = %v", err)
	}
	if remoteHead(t, bareDir, "main") != "" {
		t.Fatal("seeded a main next to their trunk")
	}
}

// A push the remote refuses fails the start with a reason a person can act
// on, and the next start finishes the job once the remote accepts.
func TestPrepareRetriesAFailedPush(t *testing.T) {
	ctx := context.Background()
	bareDir, url := bare(t)
	hook := filepath.Join(bareDir, "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\necho read only >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "overlay")
	_, err := Prepare(ctx, dir, url, "main", starterFiles, "seed")
	if err == nil || !strings.Contains(err.Error(), "write access") {
		t.Fatalf("err = %v", err)
	}
	if remoteHead(t, bareDir, "main") != "" {
		t.Fatal("remote has main after a refused push")
	}

	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	said, err := Prepare(ctx, dir, url, "main", starterFiles, "seed")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(said, "pushed the history") || remoteHead(t, bareDir, "main") != gitT(t, dir, "rev-parse", "HEAD") {
		t.Fatalf("second run: %q", said)
	}
	if commits(t, dir) != 1 {
		t.Fatal("seeded twice")
	}
}

// A repository that started without a remote (the chart's old self-seed)
// takes its history along when it gets one.
func TestPrepareAdoptsALocalRepository(t *testing.T) {
	ctx := context.Background()
	bareDir, url := bare(t)
	dir := filepath.Join(t.TempDir(), "overlay")
	if err := Seed(ctx, dir, starterFiles, "local"); err != nil {
		t.Fatal(err)
	}
	local := gitT(t, dir, "rev-parse", "HEAD")
	if _, err := Prepare(ctx, dir, url, "main", starterFiles, "seed"); err != nil {
		t.Fatal(err)
	}
	if remoteHead(t, bareDir, "main") != local {
		t.Fatal("local history not pushed")
	}
	if got := gitT(t, dir, "remote", "get-url", "origin"); got != url {
		t.Fatalf("origin = %q", got)
	}

	// A moved repository: the address is updated, not appended.
	moved := filepath.Join(t.TempDir(), "moved.git")
	if err := os.Rename(bareDir, moved); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(ctx, dir, "file://"+moved, "main", starterFiles, "seed"); err != nil {
		t.Fatal(err)
	}
	if got := gitT(t, dir, "remote", "get-url", "origin"); got != "file://"+moved {
		t.Fatalf("origin = %q after the move", got)
	}
}

func TestPrepareRefusals(t *testing.T) {
	ctx := context.Background()
	_, url := bare(t)
	if _, err := Prepare(ctx, filepath.Join(t.TempDir(), "o"), "ssh://git@example.com/o.git", "main", starterFiles, "s"); err == nil {
		t.Fatal("ssh accepted")
	}
	stray := t.TempDir()
	if err := os.WriteFile(filepath.Join(stray, "notes.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(ctx, stray, url, "main", starterFiles, "s"); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("existing non-repo dir: %v", err)
	}
	// A file where the repository should be is reported as what it is, and
	// left alone.
	file := filepath.Join(t.TempDir(), "o")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(ctx, file, url, "main", starterFiles, "s"); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("a file as --repo: %v", err)
	}
	gone := filepath.Join(t.TempDir(), "o")
	if _, err := Prepare(ctx, gone, "file:///nonexistent/remote.git", "main", starterFiles, "s"); err == nil {
		t.Fatal("unreachable remote accepted")
	}
	if _, err := os.Stat(gone); err == nil {
		t.Fatal("a failed clone left a directory behind")
	}
	// A local repository whose branch exists nowhere has nothing to push.
	dir := filepath.Join(t.TempDir(), "o")
	if err := Seed(ctx, dir, starterFiles, "local"); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(ctx, dir, url, "other", starterFiles, "s"); err == nil || !strings.Contains(err.Error(), `branch "other"`) {
		t.Fatalf("missing branch: %v", err)
	}
}

func TestGitVerb(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-C", "d", "-c", "a=b", "commit", "-q"}, "commit"},
		{[]string{"clone", "-q"}, "clone"},
		{[]string{"-C", "d"}, ""},
	} {
		if got := strings.Join(gitVerb(c.args), " "); got != c.want {
			t.Errorf("%v: %q, want %q", c.args, got, c.want)
		}
	}
}

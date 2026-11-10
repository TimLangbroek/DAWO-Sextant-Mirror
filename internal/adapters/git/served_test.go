package git

import (
	"context"
	"strings"
	"testing"
)

// refsOf lists a repository's refs as "sha ref" lines.
func refsOf(t *testing.T, dir string) map[string]string {
	t.Helper()
	refs := map[string]string{}
	for _, l := range strings.Split(run(t, dir, "for-each-ref", "--format=%(objectname) %(refname)"), "\n") {
		if sha, ref, ok := strings.Cut(l, " "); ok {
			refs[ref] = sha
		}
	}
	return refs
}

func commitIn(t *testing.T, dir, msg string) string {
	t.Helper()
	run(t, dir, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", msg)
	return run(t, dir, "rev-parse", "HEAD")
}

// With a remote, devices get what the remote has: the branch and the rings,
// and not a commit the console made but has not pushed.
func TestServedFollowsTheRemote(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	withRemote(t, dir)
	pushed := run(t, dir, "rev-parse", "HEAD")
	run(t, dir, "push", "-q", "origin", "HEAD:refs/heads/rings/pilot")
	run(t, dir, "push", "-q", "origin", "HEAD:refs/heads/proposal/x")
	run(t, dir, "fetch", "-q", "origin")
	unpushed := commitIn(t, dir, "not on the remote yet")

	r, err := Open(dir, "origin")
	if err != nil {
		t.Fatal(err)
	}
	served, err := r.Served(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := refsOf(t, served)
	if got["refs/heads/main"] != pushed {
		t.Fatalf("main served at %s, the remote has %s (unpushed %s)", got["refs/heads/main"], pushed, unpushed)
	}
	if got["refs/heads/rings/pilot"] != pushed {
		t.Fatalf("ring not served: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("served more than the branch and the rings: %v", got)
	}
	if head := run(t, served, "symbolic-ref", "HEAD"); head != "refs/heads/main" {
		t.Fatalf("served HEAD is %s", head)
	}

	// A ring the rollout removed disappears, and a pushed commit arrives.
	run(t, dir, "push", "-q", "origin", "--delete", "rings/pilot")
	run(t, dir, "push", "-q", "origin", "HEAD:main")
	if _, err := r.Served(ctx); err != nil {
		t.Fatal(err)
	}
	got = refsOf(t, served)
	if _, ok := got["refs/heads/rings/pilot"]; ok {
		t.Fatalf("a removed ring is still served: %v", got)
	}
	if got["refs/heads/main"] != unpushed {
		t.Fatalf("main served at %s after the push, want %s", got["refs/heads/main"], unpushed)
	}
}

// Without a remote the console's own branches are the fleet repository.
func TestServedWithoutARemote(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	head := commitIn(t, dir, "local only")
	r, err := Open(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	served, err := r.Served(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := refsOf(t, served)["refs/heads/main"]; got != head {
		t.Fatalf("main served at %q, want %s", got, head)
	}
}

package git

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Prepare makes dir a working copy of the fleet repository at rawURL, on
// branch, from whatever it finds:
//
//	no dir, or empty    clone the remote
//	an empty remote     fill it with files and push, so a repository just
//	                    created on any forge is all an install needs
//	an existing clone   point it at rawURL and fetch; history the remote
//	                    has never seen (a push that failed last time, a
//	                    repository that started without a remote) is pushed
//
// The credential comes from git itself, a netrc in HOME or a credential
// helper, never from an argument. The returned line says what happened.
func Prepare(ctx context.Context, dir, rawURL, branch string, files fs.FS, msg string) (string, error) {
	u, err := CheckRemoteURL(rawURL)
	if err != nil {
		return "", err
	}
	remote := u.String()
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return update(ctx, dir, remote, branch)
	}
	// An empty directory is as good as none: it is what a fresh volume or a
	// systemd tmpfiles rule hands over, and git clones into it.
	switch entries, err := os.ReadDir(dir); {
	case err == nil && len(entries) > 0:
		return "", fmt.Errorf("%s exists and is not a git repository; move it away, or point --repo at the repository you meant", dir)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return "", err
	}
	// When a clone fails git removes what it made: the directory if it
	// created it, only the contents if it was there already. Either way the
	// next start begins from the same place.
	if _, err := prepRun(ctx, "", "clone", "-q", "--", remote, dir); err != nil {
		return "", err
	}
	if hasRef(ctx, dir, "refs/remotes/origin/"+branch) {
		if _, err := prepRun(ctx, dir, "checkout", "-q", "-B", branch, "origin/"+branch); err != nil {
			return "", err
		}
		return "cloned " + remote + " (" + branch + ")", nil
	}
	if out, _ := prepRun(ctx, dir, "branch", "-r"); strings.TrimSpace(out) != "" {
		// Somebody's repository, just not on this branch. Seeding a new
		// branch next to their work would split the fleet in two.
		return "", fmt.Errorf("%s has no branch %q; set the branch the repository uses, or create it", remote, branch)
	}
	if _, err := prepRun(ctx, dir, "symbolic-ref", "HEAD", "refs/heads/"+branch); err != nil {
		return "", err
	}
	if err := copyFS(dir, files); err != nil {
		return "", err
	}
	if _, err := prepRun(ctx, dir, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := prepRun(ctx, dir, "-c", "user.name=sextant", "-c", "user.email=sextant@localhost", "commit", "-q", "-m", msg); err != nil {
		return "", err
	}
	if err := pushNew(ctx, dir, remote, branch); err != nil {
		return "", err
	}
	return remote + " was empty: filled it with a starter fleet and pushed (" + branch + ")", nil
}

// update brings an existing working copy in line with the remote.
func update(ctx context.Context, dir, remote, branch string) (string, error) {
	if _, err := prepRun(ctx, dir, "remote", "get-url", "origin"); err != nil {
		if _, err := prepRun(ctx, dir, "remote", "add", "origin", remote); err != nil {
			return "", err
		}
	} else if _, err := prepRun(ctx, dir, "remote", "set-url", "origin", remote); err != nil {
		return "", err
	}
	if _, err := prepRun(ctx, dir, "fetch", "-q", "origin"); err != nil {
		return "", err
	}
	if hasRef(ctx, dir, "refs/remotes/origin/"+branch) {
		// The console resets onto origin/<branch> at its first sync, so
		// nothing local needs moving here.
		return "up to date with " + remote + " (" + branch + ")", nil
	}
	if !hasRef(ctx, dir, "refs/heads/"+branch) {
		return "", fmt.Errorf("neither %s nor %s has a branch %q; set the branch the repository uses", dir, remote, branch)
	}
	if err := pushNew(ctx, dir, remote, branch); err != nil {
		return "", err
	}
	return remote + " had no " + branch + ": pushed the history in " + dir, nil
}

// pushNew publishes branch to a remote that does not have it yet. A refusal
// almost always means the credential can read but not write, so the error
// says that rather than leaving git's wording to be decoded.
func pushNew(ctx context.Context, dir, remote, branch string) error {
	if _, err := prepRun(ctx, dir, "push", "-q", "origin", branch); err != nil {
		return fmt.Errorf("%s has no %s yet and the push to it failed; the credential needs write access (%w)", remote, branch, err)
	}
	return nil
}

// hasRef reports whether ref resolves in dir.
func hasRef(ctx context.Context, dir, ref string) bool {
	_, err := prepRun(ctx, dir, "rev-parse", "--verify", "-q", ref)
	return err == nil
}

// prepRun runs one git command, in dir when it is set, without ever waiting
// for a password prompt.
func prepRun(ctx context.Context, dir string, args ...string) (string, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	// #nosec G204 - fixed "git" binary, argv slice (no shell); the URL was parsed and its scheme checked.
	cmd := exec.CommandContext(cctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %s", strings.Join(gitVerb(args), " "), firstLine(string(out)))
	}
	return string(out), nil
}

// gitVerb is the subcommand of a git argv, without the -C and -c options
// before it, for error messages.
func gitVerb(args []string) []string {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-C", "-c":
			i++
		default:
			return args[i : i+1]
		}
	}
	return nil
}

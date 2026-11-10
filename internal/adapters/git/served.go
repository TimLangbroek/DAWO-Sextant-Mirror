package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Served refreshes and returns a bare repository holding what devices may
// pull: the branch they follow and the rollout rings, nothing else (design
// 0014). The console serves it read only, so a private fleet repository
// needs no token on any device.
//
// WHY A COPY AND NOT THE WORKING TREE. The working tree's own branch can
// briefly hold a commit the remote does not have: the console commits, then
// pushes, and a push that loses a race is followed by a hard reset onto the
// remote. A device that fetched in between would hold a commit that then
// vanishes, and comin refuses a branch that moves backwards - the device
// would stop updating with nothing on the console to say why. So with a
// remote, the copy is made from the remote-tracking refs: only what the
// remote has accepted reaches a device. Without one (local commits only)
// the local branches are all there is.
//
// It lives inside the repository's git directory, which the console can
// always write, and is refreshed on every call; a local push with nothing
// new is a few milliseconds.
func (r *Repo) Served(ctx context.Context) (string, error) {
	r.serveMu.Lock()
	defer r.serveMu.Unlock()

	common, err := exec.CommandContext(ctx, "git", "-C", r.dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("find the git directory of %s: %w", r.dir, err)
	}
	served := filepath.Join(strings.TrimSpace(string(common)), "sextant-served.git")
	if _, err := os.Stat(filepath.Join(served, "HEAD")); err != nil {
		if out, err := exec.CommandContext(ctx, "git", "init", "-q", "--bare", served).CombinedOutput(); err != nil {
			return "", fmt.Errorf("git init %s: %s", served, strings.TrimSpace(string(out)))
		}
	}

	branch := r.branch(ctx)
	src := "refs/heads/"
	if r.remote != "" {
		src = "refs/remotes/" + r.remote + "/"
	}
	// #nosec G204 - fixed "git" binary, argv without a shell; branch and
	// remote come from the console's own repository and flags.
	out, err := exec.CommandContext(ctx, "git", "-C", r.dir, "push", "-q", "--force", "--prune", served,
		src+branch+":refs/heads/"+branch,
		src+"rings/*:refs/heads/rings/*",
	).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("refresh the served repository: %s", strings.TrimSpace(string(out)))
	}
	if out, err := exec.CommandContext(ctx, "git", "-C", served, "symbolic-ref", "HEAD", "refs/heads/"+branch).CombinedOutput(); err != nil {
		return "", fmt.Errorf("point the served repository at %s: %s", branch, strings.TrimSpace(string(out)))
	}
	return served, nil
}

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

// Seed creates dir as a new git repository holding files, committed on
// main. A dir that already holds a repository is left as it is, so a
// restart never seeds over real history; a dir that exists without one is
// refused, because writing into a directory nobody recognises could bury
// what is in it.
//
// The repository is built next to dir and renamed into place, so a failure
// halfway leaves nothing that looks like a repository to the next start.
func Seed(ctx context.Context, dir string, files fs.FS, msg string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s exists and is not a git repository; move it away, or point --repo at the repository you meant", dir)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(dir)+"-seed-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }() // nothing left to remove once renamed

	run := func(args ...string) error {
		// #nosec G204 - fixed "git" binary, argv slice (no shell).
		out, err := exec.CommandContext(ctx, "git", append([]string{"-C", tmp}, args...)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(out)))
		}
		return nil
	}
	if err := run("init", "-q", "-b", "main"); err != nil {
		return err
	}
	if err := copyFS(tmp, files); err != nil {
		return err
	}
	if err := run("add", "-A"); err != nil {
		return err
	}
	if err := run("-c", "user.name=sextant", "-c", "user.email=sextant@localhost", "commit", "-q", "-m", msg); err != nil {
		return err
	}
	return os.Rename(tmp, dir)
}

func copyFS(dst string, files fs.FS) error {
	return fs.WalkDir(files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := fs.ReadFile(files, p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

// Package starter holds the fleet repository a new console starts from
// (design 0013). One copy, embedded in the binary, so every install path
// seeds the same thing: before this, only the Helm chart seeded anything,
// from a shell heredoc, and the NixOS module and the container started
// with no repository at all.
package starter

import (
	"embed"
	"io/fs"
	"strings"
	"testing/fstest"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
)

//go:embed overlay
var overlay embed.FS

// Overlay returns the starter repository's files, rooted at the repository.
func Overlay() fs.FS {
	sub, err := fs.Sub(overlay, "overlay")
	if err != nil {
		// fs.Sub fails only on an invalid path, and "overlay" is a constant.
		panic(err)
	}
	return sub
}

// For returns the starter with its console block filled in (design 0014):
// url is the console's public address, which devices report to and pull the
// fleet repository from, and branch what a device outside every rollout ring
// follows. An empty url leaves the address out; the device module then
// refuses to build and says what is missing.
func For(url, branch string) (fs.FS, error) {
	files := fstest.MapFS{}
	err := fs.WalkDir(Overlay(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(Overlay(), name)
		if err != nil {
			return err
		}
		files[name] = &fstest.MapFile{Data: data, Mode: 0o644}
		return nil
	})
	if err != nil {
		return nil, err
	}
	f, err := fleet.Decode(files["fleet.json"].Data)
	if err != nil {
		return nil, err
	}
	f.Console = &fleet.Console{URL: strings.TrimSuffix(url, "/"), Branch: branch}
	if files["fleet.json"].Data, err = f.Encode(); err != nil {
		return nil, err
	}
	return files, nil
}

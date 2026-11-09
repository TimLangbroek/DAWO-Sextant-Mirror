// Package starter holds the fleet repository a new console starts from
// (design 0013). One copy, embedded in the binary, so every install path
// seeds the same thing: before this, only the Helm chart seeded anything,
// from a shell heredoc, and the NixOS module and the container started
// with no repository at all.
package starter

import (
	"embed"
	"io/fs"
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

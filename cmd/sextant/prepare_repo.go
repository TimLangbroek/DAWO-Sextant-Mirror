package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/adapters/git"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/starter"
)

// prepareRepo makes the fleet repository ready before the console starts:
// a clone of the remote, with the starter fleet pushed to it when the
// remote is empty, its console block naming this console. Every install
// path runs the same step, so a repository just created on GitHub,
// Codeberg, GitLab or Forgejo is all it takes.
// Credentials come from git (a netrc in HOME). It returns the exit code.
func prepareRepo(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("prepare-repo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.String("repo", "", "working copy to create or update, e.g. /data/overlay")
	url := fs.String("url", "", "the fleet repository, https://host/org/repo.git")
	branch := fs.String("branch", "main", "the branch devices follow")
	publicURL := fs.String("public-url", "", "the console's public address, written into a starter fleet for devices")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *repo == "" || *url == "" {
		_, _ = fmt.Fprintln(stderr, "usage: sextant prepare-repo --repo DIR --url https://host/org/repo.git [--branch main] [--public-url https://console.example.org]")
		return 2
	}
	files, err := starter.For(*publicURL, *branch)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "sextant:", err)
		return 1
	}
	said, err := git.Prepare(ctx, *repo, *url, *branch, files, "Start the fleet repository")
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "sextant:", err)
		return 1
	}
	_, _ = fmt.Fprintln(stdout, said)
	return 0
}

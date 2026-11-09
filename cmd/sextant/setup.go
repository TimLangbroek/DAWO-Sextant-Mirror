package main

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/adapters/git"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/forge"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/web"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/config"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/starter"
)

// setupMode reports whether the console starts in setup mode (design
// 0013). Without OIDC and without dev auth nobody can sign in, which is the
// one state in which the console is useless today; every deployment that
// works now sets one of the two and starts exactly as before.
func setupMode(cfg *config.Config) bool { return cfg.OIDCIssuer == "" && !cfg.DevAuth }

// mountSetup mounts the first-start pages and logs the one-time link. It
// reports false when setup cannot run, after saying why, and the caller
// falls back to the plain identity line.
func mountSetup(mux *http.ServeMux, cfg *config.Config, log *slog.Logger) bool {
	tok, err := web.NewSetupToken()
	if err != nil {
		log.Error("setup mode not available", "err", err)
		return false
	}
	var session string
	if len(cfg.SessionKey) > 0 {
		session = base64.StdEncoding.EncodeToString(cfg.SessionKey)
	}
	s, err := web.NewSetup(web.SetupOptions{
		DataDir:       cfg.DataDir,
		Token:         tok,
		Deployment:    cfg.Deployment,
		SecureCookies: cfg.SecureCookies,
		Locale:        cfg.DefaultLocale,
		Keys: config.Keys{SessionKey: session, SecretKey: cfg.SecretKey,
			CheckinToken: cfg.CheckinToken, OIDCClientSecret: cfg.OIDCClientSecret},
		Settings: config.Settings{PublicURL: cfg.PublicURL, RepoDir: cfg.RepoDir,
			GitRemote: cfg.GitRemote, OIDCIssuer: cfg.OIDCIssuer, OIDCClientID: cfg.OIDCClientID,
			OIDCGroupsClaim: cfg.OIDCGroupsClaim, OrgName: cfg.OrgName, OwnerSubjects: cfg.OwnerSubjects},
		SeedRepo: func(ctx context.Context, dir string) error {
			return git.Seed(ctx, dir, starter.Overlay(), "Start the fleet repository")
		},
		TestRepo:  git.LsRemote,
		CloneRepo: git.Clone,
		Log:       log,
	})
	if err != nil {
		log.Error("setup mode not available", "err", err)
		return false
	}
	s.Routes(mux)
	// The link is the credential: whoever reads the log may set the console
	// up, and the setup page tells everyone else to look here.
	log.Warn("setup: open "+setupLink(cfg, tok), "dataDir", cfg.DataDir)
	return true
}

// setupLink is the address to open. The public URL when it is known;
// otherwise the listen address, with a wildcard host shown as localhost,
// since 0.0.0.0 is not an address a browser can open.
func setupLink(cfg *config.Config, tok string) string {
	base := strings.TrimRight(cfg.PublicURL, "/")
	if base == "" {
		host, port, err := net.SplitHostPort(cfg.Addr)
		if err != nil {
			host, port = cfg.Addr, "80"
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "localhost"
		}
		base = "http://" + net.JoinHostPort(host, port)
	}
	return base + "/setup?token=" + tok
}

// forgeStore is the part of the forge identity service the handover uses.
type forgeStore interface {
	Enabled() bool
	Current(ctx context.Context) (forge.Identity, bool, error)
	Set(ctx context.Context, host, username, token, by string) error
}

// handOverSetupForge moves the forge credential setup left in keys.json
// into the console's own forge identity: sealed in the database, written to
// the netrc, and replaceable from the console (ADR 0022). Then it leaves
// keys.json. Anything that stops the move is logged and the token stays
// where it was, so the next start tries again; nothing here stops the
// console from starting.
func handOverSetupForge(ctx context.Context, id forgeStore, cfg *config.Config, log *slog.Logger) {
	sf := cfg.SetupForge
	if sf == nil {
		return
	}
	if !id.Enabled() {
		log.Warn("the forge token from setup stays in keys.json: this console has no secret key or no home directory to write a netrc to")
		return
	}
	_, stored, err := id.Current(ctx)
	if err != nil {
		log.Error("forge token from setup not moved; it stays in keys.json", "err", err)
		return
	}
	// A credential stored from the console is newer than the one setup
	// cloned with, so it wins and setup's copy simply goes.
	if !stored {
		if err := id.Set(ctx, sf.Host, sf.User, sf.Token, "setup"); err != nil {
			log.Error("forge token from setup not stored; it stays in keys.json", "err", err)
			return
		}
		log.Info("forge credential from setup stored", "host", sf.Host, "username", sf.User)
	}
	if err := config.DropForgeToken(cfg.DataDir); err != nil {
		log.Error("forge token stored, but not removed from keys.json", "err", err)
		return
	}
	cfg.SetupForge = nil
}

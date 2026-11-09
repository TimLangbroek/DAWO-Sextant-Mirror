package main

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/adapters/git"
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
		Log: log,
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

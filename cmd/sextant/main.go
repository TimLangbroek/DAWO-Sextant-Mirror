// Command sextant runs the fleet control-plane server. main is lifecycle
// only: parse config, build capabilities, mount them through the registry
// (ADR 0006), run until SIGTERM/SIGINT. All logic lives in the packages it
// belongs to.
package main

import (
	"context"
	"errors"
	"io"

	// Embed the IANA zone database: user timezone preferences must resolve
	// even in a container image without tzdata.
	_ "time/tzdata"

	"fmt"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"syscall"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/fleet"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/mw"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/capability"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/config"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/health"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/logging"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/metrics"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/platform/server"
)

// version is the release identity shown in sextant_build_info. Injected at
// image build time via -ldflags "-X main.version=..."; "dev" outside CI.
var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "check-config" {
		os.Exit(checkConfig(os.Args[2:], os.Getenv, os.Stdout, os.Stderr))
	}
	if len(os.Args) > 1 && os.Args[1] == "prepare-repo" {
		os.Exit(prepareRepo(context.Background(), os.Args[2:], os.Stdout, os.Stderr))
	}
	err := run(context.Background(), os.Args[1:], os.Getenv, os.Stderr)
	if errors.Is(err, errRestart) {
		err = restartSelf()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "sextant:", err)
		os.Exit(1)
	}
}

// checkConfig loads the configuration the way a start would and says what
// the console would run with, without starting it or writing anything. No
// secret is printed, not even whether it looks right: only whether it is
// set. It returns the exit code.
func checkConfig(args []string, getenv config.Getenv, stdout, stderr io.Writer) int {
	cfg, err := config.Load(args, getenv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "sextant:", err)
		return 1
	}
	signin := "none yet: setup mode, the console asks for it in the browser"
	switch {
	case cfg.DevAuth:
		signin = "dev-auth: anyone on this machine is owner"
	case cfg.OIDCIssuer != "":
		signin = "identity provider " + cfg.OIDCIssuer + ", returning to " + cfg.OIDCRedirectURL
	}
	repo := "none yet: setup starts one"
	if cfg.RepoDir != "" {
		repo = cfg.RepoDir
	}
	saving := "off: the console only reads"
	if cfg.Write {
		saving = "on, checked by the " + cfg.GateMode + " gate"
		if cfg.GateMode == "remote" {
			saving += " at " + cfg.GateURL
		}
	}
	db := "none: no device status, no imaging line"
	if cfg.PgDSN != "" {
		db = "set"
	}
	_, _ = fmt.Fprintf(stdout, "the console will start with this configuration\n"+
		"  listen:           %s\n  sign-in:          %s\n  fleet repository: %s\n"+
		"  saving changes:   %s\n  database:         %s\n",
		cfg.Addr, signin, repo, saving, db)
	return 0
}

// errRestart ends run so main starts the console again. Setup returns it
// once its answers are on disk.
var errRestart = errors.New("restart requested")

// restartSelf replaces this process with a fresh start of the same binary,
// arguments and environment, so everything is read again as at boot. It
// keeps the process id: a supervisor (systemd, a container runtime, the
// kubelet) sees no exit and has nothing to restart or count.
func restartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	// #nosec G702 G204 - this binary, with the arguments and environment it was started with.
	return syscall.Exec(exe, os.Args, os.Environ())
}

func run(parent context.Context, args []string, getenv config.Getenv, stderr io.Writer) error {
	cfg, err := config.Load(args, getenv)
	if err != nil {
		return err
	}
	log := logging.New(stderr, cfg.LogFormat, cfg.LogLevel)

	m := metrics.New()
	checks := health.New(5 * time.Second)
	checks.SetLogger(log)
	checks.SetBuildInfo(version, strconv.Itoa(fleet.Version), cfg.GateMode)

	ctx, stop := signal.NotifyContext(parent, syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	// Setup ends this run with errRestart; everything below shuts down as
	// it would on SIGTERM before main starts again.
	ctx, restart := context.WithCancelCause(ctx)
	defer restart(nil)

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", checks.Liveness())
	mux.Handle("GET /readyz", checks.Readiness())
	// /metrics and /status go on the main listener only when no separate one
	// is configured. The main listener is what the ingress publishes, so a
	// public console would otherwise hand out its build version, its route
	// inventory and its per-route traffic to anyone who asks.
	//
	// /status belongs with /metrics rather than with the probes: it prints the
	// exact version too, so closing one and leaving the other open moves the
	// disclosure instead of removing it. Nobody loses it - an operator sees
	// the build identity in the console footer and on the organisation page,
	// both behind a login, and a scraper reaches this listener from inside
	// the cluster.
	//
	// /healthz and /readyz stay: kubelet probes them on the container port,
	// and they answer alive/ready without saying what is running.
	if cfg.MetricsAddr == "" {
		mux.Handle("GET /status", checks.StatusPage())
		mux.Handle("GET /metrics", m.Handler())
	}

	caps, cleanup, err := buildCapabilities(ctx, cfg, log, checks, m)
	// Release anything already opened even when the build failed partway
	// (buildCapabilities returns its partial cleanup on error).
	for _, c := range cleanup {
		defer c()
	}
	if err != nil {
		return err
	}
	mounted := capability.Mount(mux, log, caps...)

	// Without a console, a console nobody can sign in to offers setup, and
	// anything else serves a plain identity line on the root.
	hooks := setupHooks{
		check:   func() error { _, err := config.Load(args, getenv); return err },
		restart: func() { restart(errRestart) },
	}
	if !slices.Contains(mounted, "console") && (!setupMode(cfg) || !mountSetup(mux, cfg, log, hooks)) {
		mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = fmt.Fprintln(w, "Sextant - declarative fleet control-plane for NixOS")
		})
	}

	handler := mw.Chain(mux,
		mw.Recover(log),
		m.Middleware,
		mw.AccessLog(log),
		mw.SecureHeaders(cfg.SecureCookies),
	)

	srv := server.New(cfg.Addr, handler, log, server.Options{ShutdownGrace: cfg.ShutdownGrace})

	if cfg.MetricsAddr != "" {
		// A second listener carrying nothing but /metrics, for an in-cluster
		// scraper. Deliberately without the middleware chain: access logging
		// every scrape drowns the log, and the metrics of the metrics
		// endpoint are noise measuring themselves.
		mmux := http.NewServeMux()
		mmux.Handle("GET /metrics", m.Handler())
		mmux.Handle("GET /status", checks.StatusPage())
		msrv := server.New(cfg.MetricsAddr, mmux, log, server.Options{ShutdownGrace: cfg.ShutdownGrace})
		mctx, mstop := context.WithCancel(ctx)
		defer mstop()
		merr := make(chan error, 1)
		go func() { merr <- msrv.Run(mctx) }()
		defer func() {
			mstop()
			// A metrics listener that failed to bind must not be silent: the
			// scraper would go blind and the graphs would simply stop, which
			// reads as "nothing is happening" rather than "nothing is being
			// measured".
			if err := <-merr; err != nil && !errors.Is(err, context.Canceled) {
				log.Error("metrics listener stopped", "err", err)
			}
		}()
	}

	runErr := srv.Run(ctx)
	if errors.Is(context.Cause(ctx), errRestart) {
		return errRestart
	}
	return runErr
}

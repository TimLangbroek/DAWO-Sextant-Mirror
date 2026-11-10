package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/cgi" // #nosec G504 - httpoxy is Go < 1.6.3, and git http-backend makes no outbound request
	"os/exec"
)

// FleetRepoAPI serves the fleet repository to enrolled devices: read only,
// over git's smart HTTP, behind the device's own credential (design 0014).
// comin pulls <console>/fleet.git with the device tag as user name and the
// credential the agent already holds as password, so a private repository
// on any forge needs no token on any device, and a retired device loses
// its pull with its credential.
//
// git itself speaks the protocol (git http-backend, as CGI); this decides
// only who may ask and what they may ask for.
type FleetRepoAPI struct {
	devs    DeviceAuthenticator
	served  func(ctx context.Context) (string, error)
	retired func(tag string) bool
	log     *slog.Logger
}

// NewFleetRepo serves the repository served returns; it is called per
// request, so the copy is refreshed before each fetch.
func NewFleetRepo(devs DeviceAuthenticator, served func(ctx context.Context) (string, error)) *FleetRepoAPI {
	return &FleetRepoAPI{devs: devs, served: served}
}

// WithLifecycle refuses retired tags even with a credential, as check-in does.
func (f *FleetRepoAPI) WithLifecycle(retired func(tag string) bool) *FleetRepoAPI {
	f.retired = retired
	return f
}

// WithLog wires a logger.
func (f *FleetRepoAPI) WithLog(l *slog.Logger) *FleetRepoAPI {
	f.log = l
	return f
}

func (f *FleetRepoAPI) logger() *slog.Logger {
	if f.log != nil {
		return f.log
	}
	return slog.Default()
}

// Routes mounts the two requests a fetch makes. Nothing else of the smart
// protocol is routed - no receive-pack, no dumb-protocol object paths - so
// a push or a crawl ends in a 404 here, before git is involved. Even a push
// that got through would land in the served copy, which the next fetch
// overwrites from the console's own repository.
func (f *FleetRepoAPI) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /fleet.git/info/refs", f.serve)
	mux.HandleFunc("POST /fleet.git/git-upload-pack", f.serve)
}

func (f *FleetRepoAPI) serve(w http.ResponseWriter, r *http.Request) {
	tag, secret, ok := r.BasicAuth()
	if !ok || !f.devs.AuthenticateTag(r.Context(), secret, tag) {
		w.Header().Set("WWW-Authenticate", `Basic realm="sextant fleet repository"`)
		http.Error(w, "the fleet repository needs a device tag and its credential", http.StatusUnauthorized)
		return
	}
	if f.retired != nil && f.retired(tag) {
		http.Error(w, "device is retired", http.StatusGone)
		return
	}
	if r.Method == http.MethodGet && r.URL.Query().Get("service") != "git-upload-pack" {
		http.Error(w, "the fleet repository is read only", http.StatusForbidden)
		return
	}
	dir, err := f.served(r.Context())
	if err != nil {
		f.logger().Error("fleet repository: cannot prepare the served copy", "err", err)
		http.Error(w, "the fleet repository is not available", http.StatusServiceUnavailable)
		return
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		f.logger().Error("fleet repository: no git on PATH", "err", err)
		http.Error(w, "the fleet repository is not available", http.StatusServiceUnavailable)
		return
	}
	h := &cgi.Handler{
		Path: gitBin,
		Args: []string{"http-backend"},
		// PATH_INFO is what follows /fleet.git, so the repository is
		// GIT_PROJECT_ROOT itself.
		Root:   "/fleet.git",
		Env:    []string{"GIT_PROJECT_ROOT=" + dir, "GIT_HTTP_EXPORT_ALL=1"},
		Logger: slog.NewLogLogger(f.logger().Handler(), slog.LevelWarn),
	}
	h.ServeHTTP(w, r)
}

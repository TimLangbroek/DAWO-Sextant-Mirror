package web

// installer_page.go: the installation USB on "Add a laptop" (design 0016).
// The gate-runner builds the image from the fleet repository; the console
// asks for it at the current revision and streams it to whoever may add a
// laptop. The runner stays internal.

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// installerName is the artifact the runner builds for the USB.
const installerName = "installer"

// installerLine is what a fleet without the output adds to its flake.
const installerLine = "packages.x86_64-linux.installer = sextant.lib.mkInstaller { inherit self; };"

var installerRevRe = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// installerView is the USB block on the page.
type installerView struct {
	Rev     string
	Phase   string // building | done | failed
	Detail  string
	File    string
	SizeMB  int64
	AddLine string
}

// mayAddLaptop: whoever may make a code for some group, or for the org, may
// also take the stick that types it in.
func (s *Server) mayAddLaptop(v view) bool {
	return len(s.editableGroups(v)) > 0 || v.roleAt("org").Meets(identity.Editor)
}

// installerState reports the USB at rev for the page, asking the runner
// (which starts nothing new for a revision it already has or is building).
func (s *Server) installerState(r *http.Request, rev string) *installerView {
	if s.svc.Installer == nil || !installerRevRe.MatchString(rev) {
		return nil
	}
	iv := &installerView{Rev: rev}
	st, err := s.svc.Installer.EnsureArtifact(r.Context(), rev, installerName)
	switch {
	case err != nil:
		iv.Phase, iv.Detail = "failed", err.Error()
	case st.Phase == ports.BuildDone:
		iv.Phase, iv.File, iv.SizeMB = "done", st.File, (st.Size+1<<20-1)>>20
	case st.Phase == ports.BuildFailed:
		iv.Phase, iv.Detail, iv.AddLine = "failed", st.Detail, installerLine
	default:
		iv.Phase = "building"
	}
	return iv
}

// postInstallerUSB asks for the stick at the revision the fleet is at now
// and goes back to the page, which shows how far it is.
func (s *Server) postInstallerUSB(w http.ResponseWriter, r *http.Request, v view) error {
	if s.svc.Installer == nil {
		return fmt.Errorf("this console has no build runner to make an installation USB")
	}
	if !s.mayAddLaptop(v) {
		// Not an editor anywhere: the org check words the refusal.
		return s.requireWeb(v, "org", identity.Editor)
	}
	rev := s.svc.Config.Head(r.Context())
	if !installerRevRe.MatchString(rev) {
		return fmt.Errorf("the fleet repository did not say which revision it is at")
	}
	if _, err := s.svc.Installer.EnsureArtifact(r.Context(), rev, installerName); err != nil {
		return err
	}
	http.Redirect(w, r, "/devices/add?usb="+url.QueryEscape(rev)+"#usb", http.StatusSeeOther)
	return nil
}

// getInstallerUSB streams a finished stick image.
func (s *Server) getInstallerUSB(w http.ResponseWriter, r *http.Request, v view) {
	rev := r.PathValue("rev")
	if s.svc.Installer == nil || !installerRevRe.MatchString(rev) {
		http.NotFound(w, r)
		return
	}
	if !s.mayAddLaptop(v) {
		http.Error(w, v.L.T("join.no_rights"), http.StatusForbidden)
		return
	}
	st, err := s.svc.Installer.EnsureArtifact(r.Context(), rev, installerName)
	if err != nil || st.Phase != ports.BuildDone {
		http.NotFound(w, r)
		return
	}
	body, err := s.svc.Installer.OpenArtifact(r.Context(), rev, installerName)
	if err != nil {
		s.log.Warn("installer download failed", "rev", rev, "err", err)
		http.Error(w, "the build runner did not hand over the image", http.StatusBadGateway)
		return
	}
	defer func() { _ = body.Close() }()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", st.File))
	if st.Size > 0 {
		w.Header().Set("Content-Length", fmt.Sprint(st.Size))
	}
	if _, err := io.Copy(w, body); err != nil {
		s.log.Info("installer download stopped", "rev", rev, "err", err)
	}
}

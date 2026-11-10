package web

import (
	"encoding/json"
	"net/http"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
)

// deviceCompliance hands out one device's compliance report (#13) as a JSON
// file: the document GET /api/v1/compliance/{tag} answers, for an auditor
// who works from the console rather than the API. Same visibility as the
// device page. The download is logged, as the CSV export is, because a read
// leaves no trail in git.
func (s *Server) deviceCompliance(w http.ResponseWriter, r *http.Request, v view) {
	tag := r.PathValue("tag")
	f := s.svc.Config.Fleet()
	if _, ok := f.Devices[tag]; !ok || !v.canView("device:"+tag) {
		http.NotFound(w, r)
		return
	}
	var st app.StatusView
	var has bool
	if s.svc.Inventory != nil {
		var err error
		// Without its status the report would say the device never reported,
		// which is a claim about the device and not about the database.
		if st, has, err = s.svc.Inventory.Status(r.Context(), tag); err != nil {
			s.log.Error("compliance report: device status", "tag", tag, "err", err)
			http.Error(w, "the device status cannot be read right now", http.StatusServiceUnavailable)
			return
		}
	}
	now := time.Now()
	rep, _ := app.NewComplianceReport(f, s.svc.Config.Profiles(), tag, st, has, s.svc.Config.Head(r.Context()), now)
	name := "compliance-" + tag + "-" + now.UTC().Format("2006-01-02") + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(rep)
	s.log.Info("compliance report exported", "by", v.User.Subject, "tag", tag)
}

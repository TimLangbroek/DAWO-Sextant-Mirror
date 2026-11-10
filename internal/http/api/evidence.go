package api

import (
	"encoding/json"
	"net/http"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// evidence.go: the audit-evidence export. Spans every scope, so org-wide
// Viewer, like the audit trail and change diffs.

// getEvidence exports the bundle for ?from=...&to=... (RFC 3339; to
// defaults to now, from to 30 days before to).
func (a *API) getEvidence(w http.ResponseWriter, r *http.Request) error {
	from, to, err := a.evidencePeriod(r)
	if err != nil {
		return err
	}
	ev, err := a.evidence.Export(r.Context(), from, to)
	if err != nil {
		return reject(err)
	}
	// An export is a document: name it so a browser download is traceable.
	w.Header().Set("Content-Disposition",
		`attachment; filename="sextant-evidence-`+from.Format("20060102")+`-`+to.Format("20060102")+`.json"`)
	writeJSON(w, http.StatusOK, ev)
	return nil
}

// getEvidenceOCSF exports the same period's audit trail as OCSF API
// Activity events for a SIEM (#17), one JSON object per line: every
// commit to the fleet repository and every reveal of a device secret,
// oldest first.
func (a *API) getEvidenceOCSF(w http.ResponseWriter, r *http.Request) error {
	from, to, err := a.evidencePeriod(r)
	if err != nil {
		return err
	}
	events, err := a.evidence.OCSF(r.Context(), from, to)
	if err != nil {
		return reject(err)
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition",
		`attachment; filename="sextant-ocsf-`+from.Format("20060102")+`-`+to.Format("20060102")+`.ndjson"`)
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			// The status is out; all that is left is to stop.
			a.log.Warn("ocsf export: write failed", "err", err)
			return nil
		}
	}
	return nil
}

// evidencePeriod checks the caller may read evidence and reads the period
// from ?from=...&to=... (RFC 3339; to defaults to now, from to 30 days
// before to).
func (a *API) evidencePeriod(r *http.Request) (from, to time.Time, err error) {
	if err := a.require(r, "org", identity.Viewer); err != nil {
		return from, to, err
	}
	if a.evidence == nil {
		return from, to, &forbidden{simpleErr("evidence export needs the change store (config plane not mounted)")}
	}
	to = a.now()
	if v := r.URL.Query().Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return from, to, &ports.ValidationError{Detail: "to: RFC 3339 expected (2026-07-01T00:00:00Z)"}
		}
		to = t
	}
	from = to.AddDate(0, 0, -30)
	if v := r.URL.Query().Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return from, to, &ports.ValidationError{Detail: "from: RFC 3339 expected (2026-07-01T00:00:00Z)"}
		}
		from = t
	}
	return from, to, nil
}

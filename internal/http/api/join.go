package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// JoinWait is how long a join may take, gate included.
const JoinWait = 15 * time.Minute

// Joiner exchanges a join code for a device (app.JoinService).
type Joiner interface {
	Join(ctx context.Context, req app.JoinRequest) (app.Joined, error)
}

// JoinAPI lets a laptop exchange a join code for its tag and credential
// (design 0015). It is the console's one write that no session or
// credential guards: the code is the authentication. So it is mounted
// outside /api/v1, rate limited like sign-in, and every refused code is
// logged with the address it came from.
//
// Every answer is JSON with an "error" in words for the person at the
// laptop, because that person cannot see the console and the agent prints
// the answer as it is.
type JoinAPI struct {
	joins   Joiner
	console func() (url, branch string)
	addr    func(*http.Request) string
	write   bool
	log     *slog.Logger
}

// NewJoin serves the exchange. console says where the laptop goes next: the
// console's public address and the branch devices follow. write is the
// console's write mode: a join commits a device, so a read-only console
// refuses it like every other change.
func NewJoin(joins Joiner, console func() (url, branch string), addr func(*http.Request) string, write bool, log *slog.Logger) *JoinAPI {
	if log == nil {
		log = slog.Default()
	}
	return &JoinAPI{joins: joins, console: console, addr: addr, write: write, log: log}
}

// Routes mounts the one route.
func (a *JoinAPI) Routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/join", a.join)
}

// joinBody is what a laptop sends. Unknown fields are allowed: a newer agent
// may say more about itself than this console reads, and that must not stop
// it joining.
type joinBody struct {
	Code           string `json:"code"`
	Name           string `json:"name,omitempty"`
	Serial         string `json:"serial,omitempty"`
	Vendor         string `json:"vendor,omitempty"`
	Model          string `json:"model,omitempty"`
	CPU            string `json:"cpu,omitempty"`
	Virtualisation string `json:"virtualisation,omitempty"`
}

func (a *JoinAPI) join(w http.ResponseWriter, r *http.Request) {
	// The answer waits for the gate, and a cold remote gate takes minutes;
	// the server's usual write deadline would cut it off. The agent waits as
	// long (agent/src/join.rs).
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(JoinWait))
	// Before the code is looked at, so it keeps its use. The web forms and
	// /api/v1 already refused every change on a read-only console; this
	// route did not, and a code made before the switch still added a device
	// for a day.
	if !a.write {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "this console is read-only, so it adds no laptops now. " +
			"Your code was not used; ask whoever runs the console."})
		return
	}
	var in joinBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the console could not read the request: " + err.Error()})
		return
	}
	got, err := a.joins.Join(r.Context(), app.JoinRequest{
		Code: in.Code, Name: in.Name, Serial: in.Serial, Vendor: in.Vendor,
		Model: in.Model, CPU: in.CPU, Virtualisation: in.Virtualisation,
	})
	var refused *app.JoinRefusal
	var gate *ports.ValidationError
	switch {
	case errors.Is(err, app.ErrCodeRefused):
		a.log.Warn("join: code refused", "addr", a.addr(r))
		writeJSON(w, http.StatusForbidden, map[string]string{"error": app.ErrCodeRefused.Reason})
		return
	case errors.As(err, &refused):
		a.log.Info("join: refused", "addr", a.addr(r), "serial", in.Serial, "reason", refused.Reason)
		writeJSON(w, http.StatusConflict, map[string]string{"error": refused.Reason})
		return
	case errors.As(err, &gate):
		a.log.Warn("join: the gate refused the new device", "serial", in.Serial, "detail", gate.Detail)
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "the console's check refused this laptop's configuration: " + gate.Detail +
			". Nothing was changed; the code still works once an administrator has fixed this."})
		return
	case err != nil:
		a.log.Error("join failed", "serial", in.Serial, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "the console could not finish the join. Try again; if it keeps failing, tell your administrator."})
		return
	}
	a.log.Info("join: a laptop joined", "tag", got.Tag, "serial", in.Serial, "addr", a.addr(r))
	url, branch := a.console()
	writeJSON(w, http.StatusCreated, map[string]any{
		"tag":        got.Tag,
		"credential": got.Credential,
		"console":    map[string]string{"url": url, "branch": branch},
	})
}

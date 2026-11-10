package web

// join_page.go: "Add a laptop" (design 0015), the one place a laptop's
// enrolment starts. It asks one question - the group - and answers with a
// code to type on the laptop. More than one laptop is a number on the same
// form, not a different kind of code.
//
// The code is shown once, in the answer to the POST that made it. It is not
// stored (only its HMAC is), so there is nothing to show again; someone who
// navigates away makes a new one and withdraws the old.

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/joincode"
)

// joinCodeRow is one open code in the list.
type joinCodeRow struct {
	ID     string
	Group  string
	Tag    string
	Uses   int
	MadeBy string
	Until  string
}

// shownCode is the code just made, for the one page that shows it.
type shownCode struct {
	Code    string
	Group   string
	Tag     string
	Uses    int
	Until   string
	Command string
}

const joinTimeFormat = "Mon 2 Jan 15:04"

func (s *Server) addLaptopPage(w http.ResponseWriter, r *http.Request, v view) {
	s.renderAddLaptop(w, r, v, nil)
}

// editableGroups are the groups this person may make a code for.
func (s *Server) editableGroups(v view) []string {
	var out []string
	for g := range s.svc.Config.Fleet().Groups {
		if v.roleAt("group:" + g).Meets(identity.Editor) {
			out = append(out, g)
		}
	}
	sort.Strings(out)
	return out
}

func (s *Server) renderAddLaptop(w http.ResponseWriter, r *http.Request, v view, shown *shownCode) {
	if s.svc.Joins == nil {
		s.unavailable(w, r, v, v.L.T("degraded.needs_store"))
		return
	}
	groups := s.editableGroups(v)
	orgEditor := v.roleAt("org").Meets(identity.Editor)
	if len(groups) == 0 && !orgEditor {
		http.Error(w, v.L.T("join.no_rights"), http.StatusForbidden)
		return
	}
	open, err := s.svc.Joins.Open(r.Context())
	if err != nil {
		s.log.Warn("list join codes failed", "err", err)
	}
	rows := make([]joinCodeRow, 0, len(open))
	for _, c := range open {
		scope := "org"
		switch {
		case c.Tag != "":
			scope = "device:" + c.Tag
		case c.Group != "":
			scope = "group:" + c.Group
		}
		if !v.canView(scope) {
			continue
		}
		rows = append(rows, joinCodeRow{ID: c.ID, Group: c.Group, Tag: c.Tag, Uses: c.Uses,
			MadeBy: c.CreatedBy, Until: c.Expires.Local().Format(joinTimeFormat)})
	}
	s.render(w, "add_laptop", map[string]any{
		"Title": v.L.T("join.title"), "Nav": "devices",
		"Groups": groups, "OrgEditor": orgEditor,
		"Codes": rows, "Shown": shown, "MaxUses": joincode.MaxUses,
	}, v)
}

// madeBy is how the list names the person who made a code.
func madeBy(v view) string {
	for _, s := range []string{v.User.Name, v.User.Email, v.User.Subject} {
		if s != "" {
			return s
		}
	}
	return "?"
}

func (s *Server) shown(code string, c joincode.Code) *shownCode {
	url := ""
	if s.svc.JoinConsole != nil {
		url, _ = s.svc.JoinConsole()
	}
	if url == "" {
		url = "https://<console>"
	}
	return &shownCode{
		Code: code, Group: c.Group, Tag: c.Tag, Uses: c.Uses,
		Until:   c.Expires.Local().Format(joinTimeFormat),
		Command: fmt.Sprintf("sudo sextant-agent join %s %s", url, code),
	}
}

func (s *Server) postAddLaptop(w http.ResponseWriter, r *http.Request, v view) error {
	if s.svc.Joins == nil {
		return fmt.Errorf("%s", v.L.T("degraded.needs_store"))
	}
	group := r.FormValue("group")
	scope := "org"
	if group != "" {
		scope = "group:" + group
	}
	if err := s.requireWeb(v, scope, identity.Editor); err != nil {
		return err
	}
	uses := 1
	if u := r.FormValue("uses"); u != "" {
		n, err := strconv.Atoi(u)
		if err != nil || n < 1 {
			return fmt.Errorf("number of laptops: %q is not a number from 1 to %d", u, joincode.MaxUses)
		}
		uses = n
	}
	code, c, err := s.svc.Joins.Create(r.Context(), app.NewCode{Group: group, Uses: uses, By: madeBy(v)})
	if err != nil {
		return err
	}
	s.log.Info("join code made", "id", c.ID, "group", group, "uses", uses, "by", v.User.Subject)
	s.renderAddLaptop(w, r, v, s.shown(code, c))
	return nil
}

// postDeviceReenrol makes a code for one existing device: the laptop that
// presents it gets a new credential and stays the same device.
func (s *Server) postDeviceReenrol(w http.ResponseWriter, r *http.Request, v view) error {
	if s.svc.Joins == nil {
		return fmt.Errorf("%s", v.L.T("degraded.needs_store"))
	}
	tag := r.PathValue("tag")
	if err := s.requireDeviceEditor(v, tag); err != nil {
		return err
	}
	code, c, err := s.svc.Joins.Create(r.Context(), app.NewCode{Tag: tag, By: madeBy(v)})
	if err != nil {
		return err
	}
	s.log.Info("re-enrol code made", "id", c.ID, "tag", tag, "by", v.User.Subject)
	s.renderAddLaptop(w, r, v, s.shown(code, c))
	return nil
}

// postWithdrawJoinCode closes a code before its time. Whoever may make a
// code for its group (or device) may withdraw it.
func (s *Server) postWithdrawJoinCode(w http.ResponseWriter, r *http.Request, v view) error {
	if s.svc.Joins == nil {
		return fmt.Errorf("%s", v.L.T("degraded.needs_store"))
	}
	id := r.PathValue("id")
	open, err := s.svc.Joins.Open(r.Context())
	if err != nil {
		return err
	}
	for _, c := range open {
		if c.ID != id {
			continue
		}
		var err error
		switch {
		case c.Tag != "":
			err = s.requireDeviceEditor(v, c.Tag)
		case c.Group != "":
			err = s.requireWeb(v, "group:"+c.Group, identity.Editor)
		default:
			err = s.requireWeb(v, "org", identity.Editor)
		}
		if err != nil {
			return err
		}
		if err := s.svc.Joins.Withdraw(r.Context(), id); err != nil {
			return err
		}
		s.log.Info("join code withdrawn", "id", id, "by", v.User.Subject)
		break
	}
	http.Redirect(w, r, "/devices/add", http.StatusSeeOther)
	return nil
}

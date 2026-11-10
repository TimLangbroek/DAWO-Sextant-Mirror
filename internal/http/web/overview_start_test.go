package web_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/http/web"
)

func overviewOf(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	resp, err := client().Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

// A new console opens on what it still needs, not on a page of zeros: each
// open step says what it is for and links to where it is done, and the list
// is gone once everything is done.
func TestTheOverviewSaysWhatANewConsoleStillNeeds(t *testing.T) {
	// Read-only and without a database: both say so, and the first group is
	// already there in this fleet.
	page := overviewOf(t, consoleWithWrite(t, false))
	for _, want := range []string{`data-step-done="start.signin"`, `data-step-open="start.changes"`, `data-step-open="start.database"`, `data-step-done="start.group"`} {
		if !strings.Contains(page, want) {
			t.Errorf("read-only console without a database: no %s", want)
		}
	}

	// Everything but a laptop: the last step links to Add a laptop.
	empty := `{"version": 3, "groups": {"pilot": {}}, "devices": {}}`
	ts, _, _, _ := joinConsoleAs(t, true, empty, web.DevSessions{})
	page = overviewOf(t, ts)
	if !strings.Contains(page, `data-step-open="start.laptop"><a href="/devices/add">`) {
		t.Error("the laptop step does not lead to Add a laptop")
	}
	for _, key := range []string{"changes", "database", "group"} {
		if !strings.Contains(page, `data-step-done="start.`+key+`"`) {
			t.Errorf("step %s is not done on a writing console with a database and a group", key)
		}
	}

	// No group yet: that step links to Groups.
	ts, _, _, _ = joinConsoleAs(t, true, `{"version": 3, "groups": {}, "devices": {}}`, web.DevSessions{})
	if page = overviewOf(t, ts); !strings.Contains(page, `data-step-open="start.group"><a href="/groups">`) {
		t.Error("the group step does not lead to Groups")
	}

	// All done: no list.
	done := `{"version": 3, "groups": {"pilot": {}}, "devices": {"lt-1": {"groups": ["pilot"], "hardware": "hw"}}}`
	ts, _, _, _ = joinConsoleAs(t, true, done, web.DevSessions{})
	if strings.Contains(overviewOf(t, ts), "data-getting-started") {
		t.Error("the list stays after everything is done")
	}

	// And it is for whoever can do the steps: someone who sees the whole
	// organisation but edits nothing gets the overview without it.
	orgView := `{"version": 3, "groups": {}, "devices": {},
	  "access": [{"group": "org-view", "role": "viewer", "scope": "org"}]}`
	viewer := identity.User{Subject: "v", Groups: []string{"org-view"}}
	ts, _, _, _ = joinConsoleAs(t, true, orgView, scopedSessions{viewer})
	page = overviewOf(t, ts)
	if !strings.Contains(page, "</html>") || strings.Contains(page, `id="start-title"`) {
		t.Error("an organisation viewer is shown steps they cannot take")
	}
}

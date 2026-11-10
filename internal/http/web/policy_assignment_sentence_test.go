package web_test

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// Who gets a policy, and why, reads off one line (#33). The line used to
// name its filter and stop there, so the reader went looking for a second
// object further down the page to learn what "laptops" meant. It now says
// what the filter selects, joined the way the filter joins its rules.
func TestAnAssignmentSaysWhatItsFilterSelects(t *testing.T) {
	ts, _ := newConsole(t)
	post := func(path string, form url.Values) {
		t.Helper()
		form.Set("csrf", "dev-csrf")
		resp, err := client().PostForm(ts.URL+path, form)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("POST %s = %d", path, resp.StatusCode)
		}
	}
	post("/policies", url.Values{"id": {"baseline"}, "settings": {"desktop = plasma"}})
	post("/policies", url.Values{"id": {"office"}, "settings": {"desktop = gnome"}})
	post("/filters", url.Values{"id": {"portable"}, "match": {"any"},
		"attr0": {"class"}, "op0": {"eq"}, "value0": {"laptop"},
		"attr1": {"hardware"}, "op1": {"in"}, "value1": {"t495s, msi"}})
	post("/filters", url.Values{"id": {"pilots"}, "match": {"all"},
		"attr0": {"class"}, "op0": {"eq"}, "value0": {"laptop"},
		"attr1": {"group"}, "op1": {"eq"}, "value1": {"pilot"}})
	post("/assignments", url.Values{"policy": {"baseline"}, "target": {"org"}, "filter": {"portable"}})
	post("/assignments", url.Values{"policy": {"office"}, "target": {"org"}, "filter": {"pilots"}})
	post("/assignments", url.Values{"policy": {"office"}, "target": {"group:pilot"}})

	resp, err := client().Get(ts.URL + "/policies")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	page := string(b)

	got := regexp.MustCompile(`<code[^>]*data-filter-rules[^>]*>([^<]*)</code>`).FindAllStringSubmatch(page, -1)
	lines := make([]string, 0, len(got))
	for _, m := range got {
		lines = append(lines, m[1])
	}
	want := []string{
		"class eq laptop or hardware in t495s, msi",
		"class eq laptop and group eq pilot",
	}
	if strings.Join(lines, "|") != strings.Join(want, "|") {
		t.Fatalf("the assignment lines say %q, want %q", lines, want)
	}
}

package web_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// A condition was a first-class part of a policy that only the API could
// write (#77), and the console's editor was worse than missing: it built
// the policy from the fields it had, so saving a policy here dropped every
// condition the API had set. The editor now carries a policy's conditions,
// a policy can be only conditions, and a form without condition rows
// leaves them alone.
func TestPolicyConditionsFromTheConsole(t *testing.T) {
	ts, cfg := newConsole(t)
	post := func(form url.Values) (int, string) {
		t.Helper()
		form.Set("csrf", "dev-csrf")
		resp, err := client().PostForm(ts.URL+"/policies", form)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}

	// A policy that only requires something: no settings at all.
	if code, body := post(url.Values{"id": {"disk"}, "description": {"Room to update"},
		"cmetric0": {"disk.free_percent"}, "cop0": {">="}, "cvalue0": {"15"},
		"cdetail0": {"An update needs room to unpack"}}); code != http.StatusSeeOther {
		t.Fatalf("a policy of one condition = %d %s", code, body)
	}
	p := cfg.Fleet().Policies["disk"]
	if len(p.Settings) != 0 || len(p.Conditions) != 1 {
		t.Fatalf("policy = %+v", p)
	}
	if c := p.Conditions[0]; c.Metric != "disk.free_percent" || c.Op != ">=" || c.Value != 15 ||
		c.Detail != "An update needs room to unpack" {
		t.Fatalf("condition = %+v", c)
	}

	// The page shows it, and its editor holds it plus an empty row.
	resp, err := client().Get(ts.URL + "/policies")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	page := string(b)
	for _, want := range []string{`data-condition`, `disk.free_percent &gt;= 15`,
		`name="cmetric0" value="disk.free_percent"`, `name="cvalue0" value="15"`,
		`name="cmetric1" value=""`, `<option value="disk.free_percent">`} {
		if !strings.Contains(page, want) {
			t.Errorf("the policies page lacks %s", want)
		}
	}

	// Edit: change the threshold, add a second condition and a setting.
	if code, body := post(url.Values{"id": {"disk"}, "settings": {"desktop = plasma"},
		"cmetric0": {"disk.free_percent"}, "cop0": {">="}, "cvalue0": {"20"},
		"cmetric1": {"memory.total_mb"}, "cop1": {">="}, "cvalue1": {"8192"}}); code != http.StatusSeeOther {
		t.Fatalf("edit = %d %s", code, body)
	}
	p = cfg.Fleet().Policies["disk"]
	if len(p.Conditions) != 2 || p.Conditions[0].Value != 20 || p.Conditions[1].Metric != "memory.total_mb" ||
		p.Settings["desktop"] != "plasma" {
		t.Fatalf("after the edit = %+v", p)
	}

	// A form that has no condition rows - a script, an old page - keeps them.
	if code, body := post(url.Values{"id": {"disk"}, "settings": {"desktop = gnome"}}); code != http.StatusSeeOther {
		t.Fatalf("edit without rows = %d %s", code, body)
	}
	if p = cfg.Fleet().Policies["disk"]; len(p.Conditions) != 2 {
		t.Fatalf("a form without condition rows dropped the conditions: %+v", p.Conditions)
	}

	// An emptied row removes its condition.
	if code, body := post(url.Values{"id": {"disk"}, "settings": {"desktop = gnome"},
		"cmetric0": {""}, "cvalue0": {"20"},
		"cmetric1": {"memory.total_mb"}, "cop1": {">="}, "cvalue1": {"8192"}}); code != http.StatusSeeOther {
		t.Fatalf("remove one = %d %s", code, body)
	}
	if p = cfg.Fleet().Policies["disk"]; len(p.Conditions) != 1 || p.Conditions[0].Metric != "memory.total_mb" {
		t.Fatalf("after emptying a row = %+v", p.Conditions)
	}

	// Refusals say why, and change nothing.
	for name, form := range map[string]url.Values{
		"not a number":   {"id": {"disk"}, "cmetric0": {"disk.free_percent"}, "cop0": {">="}, "cvalue0": {"lots"}},
		"no such metric": {"id": {"disk"}, "cmetric0": {"disk.happiness"}, "cop0": {">="}, "cvalue0": {"1"}},
		"says nothing":   {"id": {"empty"}, "cmetric0": {""}},
	} {
		code, body := post(form)
		if code < 400 || code > 499 {
			t.Errorf("%s: %d, want a refusal", name, code)
		}
		if !strings.Contains(body, map[string]string{"not a number": "is not a number",
			"no such metric": "unknown metric", "says nothing": "says nothing"}[name]) {
			t.Errorf("%s: the refusal does not say why:\n%.400s", name, body)
		}
	}
	if p = cfg.Fleet().Policies["disk"]; len(p.Conditions) != 1 {
		t.Errorf("a refused edit changed the policy: %+v", p.Conditions)
	}
}

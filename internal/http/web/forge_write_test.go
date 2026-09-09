package web_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
)

// forge_write_test.go: replacing and dropping the credential the console
// pushes with (ADR 0022).
//
// The page has tests. The two handlers that WRITE had none, and they are the
// half that matters: this credential is what makes a config commit arrive as
// a machine account instead of as a person, which is the whole of audit
// finding H2. A save reachable by a non-owner hands the fleet's write key to
// whoever can reach the form.

func forgePost(t *testing.T, ts, path, csrf string, form url.Values) *http.Response {
	t.Helper()
	form.Set("csrf", csrf)
	resp, err := client().PostForm(ts+path, form)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestOnlyAnOwnerReplacesTheForgeCredential(t *testing.T) {
	// alpha-team is granted viewer at group:alpha by the seed, so this user
	// gets INTO the console and is refused by the handler's own check rather
	// than by the door. A user with no grant at all never reaches the handler,
	// which is why an earlier version of this test passed with the owner
	// check removed.
	ts, svc := newForgeConsole(t, identity.User{Subject: "u", Groups: []string{"alpha-team"}})

	resp := forgePost(t, ts.URL, "/org/forge", "csrf", url.Values{
		"host": {"forge.example.org"}, "username": {"intruder"}, "token": {"t"}})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode == http.StatusSeeOther {
		t.Fatalf("a non-owner replaced the credential the console pushes with: %s", body)
	}
	if _, ok, err := svc.Current(context.Background()); err != nil || ok {
		t.Error("a credential was stored by somebody who may not")
	}

	clear := forgePost(t, ts.URL, "/org/forge/clear", "csrf", url.Values{})
	clear.Body.Close()
	if clear.StatusCode == http.StatusSeeOther {
		t.Error("a non-owner cleared the credential; the console falls back to whatever is mounted")
	}
}

func TestSavingTheForgeCredentialWritesWhatGitReads(t *testing.T) {
	ts, svc := newForgeConsole(t, identity.User{Subject: "u", Name: "Bram", Groups: []string{"owners"}})

	const secret = "tok-saved-through-the-form"
	resp := forgePost(t, ts.URL, "/org/forge", "csrf", url.Values{
		"host": {"forge.example.org"}, "username": {"sextant-bot"}, "token": {"  " + secret + "  "}})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save = %d, want 303: %s", resp.StatusCode, body)
	}
	if strings.Contains(string(body), secret) {
		t.Error("the response carried the token back")
	}
	if loc := resp.Header.Get("Location"); strings.Contains(loc, secret) {
		t.Errorf("the token is in the redirect: %s", loc)
	}

	id, ok, err := svc.Current(context.Background())
	if err != nil || !ok {
		t.Fatalf("nothing stored after a successful save: %v %v", ok, err)
	}
	if id.Username != "sextant-bot" || id.Host != "forge.example.org" {
		t.Errorf("stored identity = %+v", id)
	}
	// Whitespace from a paste is trimmed; the token itself is not touched.
	// A netrc line with a stray space authenticates as a wrong password, and
	// the operator debugs the forge instead of the paste.
	if id.UpdatedBy != "Bram" {
		t.Errorf("the trail does not say who rotated it: %+v", id)
	}
}

// TestClearingTheForgeCredentialReturnsToTheMount: clearing is how a
// deployment goes back to the credential its packaging provides, so it has to
// actually remove the stored one.
func TestClearingTheForgeCredentialReturnsToTheMount(t *testing.T) {
	ts, svc := newForgeConsole(t, identity.User{Subject: "u", Name: "Bram", Groups: []string{"owners"}})
	if err := svc.Set(context.Background(), "forge.example.org", "sextant-bot", "tok", "Bram"); err != nil {
		t.Fatal(err)
	}

	resp := forgePost(t, ts.URL, "/org/forge/clear", "csrf", url.Values{})
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("clear = %d, want 303", resp.StatusCode)
	}
	if _, ok, err := svc.Current(context.Background()); err != nil || ok {
		t.Error("the credential survived a clear")
	}
}

// TestForgeWritesWithoutADatabaseSayWhy: ForgeID is nil without postgres and
// both handlers dereference it.
func TestForgeWritesWithoutADatabaseSayWhy(t *testing.T) {
	ts, _ := newConsole(t)
	for _, path := range []string{"/org/forge", "/org/forge/clear"} {
		resp := forgePost(t, ts.URL, path, "dev-csrf", url.Values{"host": {"h"}, "username": {"u"}, "token": {"t"}})
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusSeeOther {
			t.Errorf("%s reported success with no store behind it", path)
		}
		if !strings.Contains(strings.ToLower(string(body)), "postgres") {
			t.Errorf("%s does not name what is missing: %s", path, body)
		}
	}
}

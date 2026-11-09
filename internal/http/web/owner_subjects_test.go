package web_test

import (
	"net/http"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
)

// The console lets an owner by subject in with no group, and still turns
// away a subject that is not on the list.
func TestOwnerSubjectGetsIntoTheConsole(t *testing.T) {
	get := func(u identity.User) int {
		ts := newScopedConsoleWith(t, u, []string{"sub-setup"})
		resp, err := http.Get(ts.URL + "/devices")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := get(identity.User{Subject: "sub-setup", Name: "Setup Person"}); code != http.StatusOK {
		t.Fatalf("owner by subject: %d, want 200", code)
	}
	if code := get(identity.User{Subject: "sub-other"}); code != http.StatusForbidden {
		t.Fatalf("other subject: %d, want 403", code)
	}
}

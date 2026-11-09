package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
)

// An owner by subject writes at org through the API with no group at all,
// which is how the person who completed setup works before any group is
// mapped (design 0013).
func TestOwnerSubjectThroughTheAPI(t *testing.T) {
	svc, _ := seededService(t, rbacSeed)
	sessions := &fakeSessions{csrf: "test-csrf", users: map[string]identity.User{
		"setup": {Subject: "sub-setup"},
		"other": {Subject: "sub-other"},
	}}
	mux := http.NewServeMux()
	New(Services{Config: svc}, Authz{Sessions: sessions, OwnerSubjects: []string{"sub-setup"}}, "", true,
		discardLog()).Routes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if code, body := sessionCall(t, srv, "setup", "POST", "/api/v1/settings",
		map[string]any{"scope": "org", "key": "apps.office", "value": true}, "test-csrf"); code != 200 {
		t.Fatalf("owner by subject at org = %d, want 200: %v", code, body)
	}
	if code, _ := sessionCall(t, srv, "other", "GET", "/api/v1/devices", nil, ""); code != 403 {
		t.Fatalf("other subject read = %d, want 403", code)
	}
}

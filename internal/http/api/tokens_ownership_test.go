package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
	tokenpkg "code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/token"
)

// tokens_ownership_test.go: who may revoke whose token.
//
// The rule is one line of comment in tokens.go - a user manages their OWN
// tokens, an org owner manages everybody's - and it was carried by
// getTokens, deleteToken and tokenOwned, all three at 0%. A revocation that
// reaches somebody else's token is an account takeover with an audit trail
// that says the owner did it.

// tokenAPI serves the token routes with two principals: a viewer and an
// org owner, each holding one token of their own.
// tokenSeed grants owner AT ORG, which is what deleteToken requires for
// somebody else's token. rbacSeed's owner is owner at group:stad, which
// is deliberately not enough here.
const tokenSeed = `{
  "version": 3,
  "org": {"settings": {"desktop": "plasma"}},
  "groups": {"stad": {}},
  "devices": {"lt-1": {"groups": ["stad"], "hardware": "hw"}},
  "access": [
    {"group": "org-owners", "role": "owner", "scope": "org"},
    {"group": "auditors", "role": "viewer", "scope": "org"}
  ]
}`

func tokenAPI(t *testing.T) (*httptest.Server, *app.TokenService) {
	t.Helper()
	svc, _ := seededService(t, tokenSeed)
	store := newAPIMemTokenStore()
	toks := app.NewTokenService(store, tickClock{}, 0)

	// One token per subject, minted through the service so the store holds
	// exactly what production would.
	for _, m := range []struct{ id, subject string }{{"vw-own", "vw"}, {"ow-own", "ow"}} {
		if _, _, err := toks.Mint(context.Background(), app.MintRequest{
			ID: m.id, Name: m.id, Kind: tokenpkg.Personal, Subject: m.subject,
		}); err != nil {
			t.Fatalf("mint %s: %v", m.id, err)
		}
	}

	ta := &fakeTokenAuth{users: map[string]identity.User{
		"as-viewer": {Subject: "vw", Name: "Vera", Groups: []string{"auditors"}},
		"as-owner":  {Subject: "ow", Name: "Oscar", Groups: []string{"org-owners"}},
	}, ceiling: map[string]identity.Role{}}

	mux := http.NewServeMux()
	New(Services{Config: svc, Tokens: toks}, Authz{Tokens: ta},
		testToken, true, discardLog()).Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, toks
}

func tokenCall(t *testing.T, srv *httptest.Server, method, path, bearer string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	return resp.StatusCode, string(buf[:n])
}

func TestAViewerRevokesOnlyTheirOwnToken(t *testing.T) {
	srv, toks := tokenAPI(t)

	// Somebody else's token: refused, and it survives.
	if code, _ := tokenCall(t, srv, "DELETE", "/api/v1/tokens/ow-own", "as-viewer"); code != http.StatusForbidden {
		t.Errorf("viewer revoking the owner's token = %d, want 403", code)
	}
	if list, err := toks.List(context.Background(), "ow"); err != nil || len(list) != 1 {
		t.Errorf("the owner's token did not survive a refused revocation: %v %v", list, err)
	}

	// Their own: allowed.
	if code, body := tokenCall(t, srv, "DELETE", "/api/v1/tokens/vw-own", "as-viewer"); code != http.StatusOK {
		t.Errorf("viewer revoking their own token = %d, want 200 (%s)", code, body)
	}
	if list, _ := toks.List(context.Background(), "vw"); len(list) != 0 {
		t.Errorf("the viewer's own token survived their own revocation: %v", list)
	}
}

func TestAnOrgOwnerRevokesAnybodysToken(t *testing.T) {
	srv, toks := tokenAPI(t)
	if code, body := tokenCall(t, srv, "DELETE", "/api/v1/tokens/vw-own", "as-owner"); code != http.StatusOK {
		t.Errorf("owner revoking another subject's token = %d, want 200 (%s)", code, body)
	}
	if list, _ := toks.List(context.Background(), "vw"); len(list) != 0 {
		t.Errorf("the token survived an owner's revocation: %v", list)
	}
}

// TestListingTokensShowsOnlyYoursAndNeverAHash covers both halves of the
// listing: the scope, and the one field that must never leave the process.
func TestListingTokensShowsOnlyYoursAndNeverAHash(t *testing.T) {
	srv, _ := tokenAPI(t)

	code, body := tokenCall(t, srv, "GET", "/api/v1/tokens", "as-viewer")
	if code != http.StatusOK {
		t.Fatalf("list = %d, want 200 (%s)", code, body)
	}
	if !strings.Contains(body, "vw-own") {
		t.Errorf("the viewer cannot see their own token: %s", body)
	}
	if strings.Contains(body, "ow-own") {
		t.Errorf("the listing leaked another subject's token: %s", body)
	}
	for _, forbidden := range []string{`"hash"`, `"Hash"`} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the listing carries %s; a stored hash must never leave the process: %s", forbidden, body)
		}
	}
}

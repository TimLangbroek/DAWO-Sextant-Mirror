package oidc

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/domain/identity"
)

// Identify says who signed in and nothing else: no session cookie, and the
// Authorize gate is not asked, because setup shows the result to an
// operator before any role exists that would let them in.
func TestIdentifySetsNoSessionAndAsksNoGate(t *testing.T) {
	idp := newFakeIDP(t)
	a := newTestAuthenticator(t, idp)
	asked := false
	a.authorize = func(identity.User) bool { asked = true; return false }

	rec, state, nonce := login(t, a)
	idp.setIDTokenClaims(map[string]any{"nonce": nonce, "sub": "sub-setup", "name": "Setup Person", "groups": "admins"})
	req := httptest.NewRequest(http.MethodGet, "/callback?state="+state+"&code=c", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	out := httptest.NewRecorder()
	u, err := a.Identify(out, req)
	if err != nil {
		t.Fatalf("Identify: %v", err)
	}
	if u.Subject != "sub-setup" || u.Name != "Setup Person" || len(u.Groups) != 1 || u.Groups[0] != "admins" {
		t.Fatalf("user = %+v", u)
	}
	if asked {
		t.Fatal("Identify asked the Authorize gate")
	}
	if sessionCookie(out.Result().Cookies()) != nil {
		t.Fatal("Identify set a session cookie")
	}
}

// A failed flow comes back as a FlowError with a status and a message safe
// for the browser, which is what setup shows.
func TestIdentifyFailureIsAFlowError(t *testing.T) {
	a := newTestAuthenticator(t, newFakeIDP(t))
	_, err := a.Identify(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/callback?state=x&code=c", nil))
	var fe *FlowError
	if !errors.As(err, &fe) || fe.Status != http.StatusBadRequest || fe.Msg != "login flow expired, try again" {
		t.Fatalf("err = %#v", err)
	}
}

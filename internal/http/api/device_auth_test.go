package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
)

// device_auth_test.go: this endpoint turns a device credential into a tag,
// which makes it an oracle. Its own header says so, and it had no test at all.
//
// Every case below is a way the oracle could open by accident: an empty gate
// token, a missing store, a wrong bearer, or a 401 where a 403 belongs. The
// last one is not pedantry - the gate reads the status to decide whether to
// look at its own token or the device's, and a wrong code sends whoever is
// debugging a closed cache to the wrong secret.

// deviceAuthFixture builds the endpoint with one known credential.
func deviceAuthFixture(t *testing.T, gateToken string) (*DeviceAuthAPI, string) {
	t.Helper()
	creds := app.NewDeviceCredentials(newAPIMemTokenStore(), tickClock{})
	secret, err := creds.Issue(context.Background(), "lt-1")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return NewDeviceAuth(creds, gateToken), secret
}

func deviceAuthCall(t *testing.T, a *DeviceAuthAPI, bearer, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	a.Routes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/device-auth", strings.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestDeviceAuthAnswersTheGateAndNobodyElse(t *testing.T) {
	a, secret := deviceAuthFixture(t, "gate-token")

	t.Run("the gate gets the tag", func(t *testing.T) {
		rec := deviceAuthCall(t, a, "gate-token", `{"credential":"`+secret+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		var got deviceAuthResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Tag != "lt-1" {
			t.Errorf("tag = %q, want lt-1", got.Tag)
		}
	})

	t.Run("no bearer is refused", func(t *testing.T) {
		if rec := deviceAuthCall(t, a, "", `{"credential":"`+secret+`"}`); rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401 - the oracle answered an unauthenticated caller", rec.Code)
		}
	})

	t.Run("a wrong bearer is refused", func(t *testing.T) {
		rec := deviceAuthCall(t, a, "not-the-gate", `{"credential":"`+secret+`"}`)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "lt-1") {
			t.Error("the tag leaked to a caller that did not authenticate")
		}
	})

	t.Run("an unknown credential is 403, not 401", func(t *testing.T) {
		rec := deviceAuthCall(t, a, "gate-token", `{"credential":"nope"}`)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403: the caller authenticated, the credential did not", rec.Code)
		}
	})

	t.Run("a malformed body is 400", func(t *testing.T) {
		if rec := deviceAuthCall(t, a, "gate-token", `{`); rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
		if rec := deviceAuthCall(t, a, "gate-token", `{"credential":"   "}`); rec.Code != http.StatusBadRequest {
			t.Errorf("blank credential status = %d, want 400", rec.Code)
		}
	})

	t.Run("an oversized body is refused before it is parsed", func(t *testing.T) {
		big := `{"credential":"` + strings.Repeat("a", 8<<10) + `"}`
		if rec := deviceAuthCall(t, a, "gate-token", big); rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %d, want 413", rec.Code)
		}
	})
}

// TestDeviceAuthWithoutAGateTokenIsClosed is the property the constructor's
// comment promises: a deployment that forgot the token refuses every call
// rather than quietly answering them. Mounted but closed.
func TestDeviceAuthWithoutAGateTokenIsClosed(t *testing.T) {
	a, secret := deviceAuthFixture(t, "")
	for _, bearer := range []string{"", "anything", "gate-token"} {
		rec := deviceAuthCall(t, a, bearer, `{"credential":"`+secret+`"}`)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("bearer %q got %d, want 401 - an unconfigured oracle answered", bearer, rec.Code)
		}
	}
}

// TestDeviceAuthWithoutAStoreIsClosed: nil credentials must not turn into a
// panic or an accidental answer.
func TestDeviceAuthWithoutAStoreIsClosed(t *testing.T) {
	a := NewDeviceAuth(nil, "gate-token")
	if rec := deviceAuthCall(t, a, "gate-token", `{"credential":"x"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

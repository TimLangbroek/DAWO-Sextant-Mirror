package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/app"
	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

type fakeJoiner struct {
	got  app.JoinRequest
	out  app.Joined
	err  error
	logs bytes.Buffer
}

func (f *fakeJoiner) Join(_ context.Context, req app.JoinRequest) (app.Joined, error) {
	f.got = req
	return f.out, f.err
}

func joinServer(f *fakeJoiner) http.Handler {
	mux := http.NewServeMux()
	NewJoin(f, func() (string, string) { return "https://console.example.test", "stable" },
		func(*http.Request) string { return "192.0.2.7" },
		slog.New(slog.NewTextHandler(&f.logs, nil))).Routes(mux)
	return mux
}

func postJoin(t *testing.T, h http.Handler, body string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/join", strings.NewReader(body)))
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("answer is not JSON (%d): %s", rec.Code, rec.Body)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control %q on an answer that can carry a credential", cc)
	}
	return rec.Code, out
}

func TestJoinAnswersWithTheCredentialAndWhereToGo(t *testing.T) {
	f := &fakeJoiner{out: app.Joined{Tag: "laptop-3abcde", Credential: "sxt_device-laptop-3abcde_x"}}
	code, out := postJoin(t, joinServer(f), `{"code":"k7qm-4xwd","serial":"PF3ABCDE","vendor":"LENOVO","model":"21BR",
		"cpu":"GenuineIntel","virtualisation":"none","name":"","fromANewerAgent":true}`)
	if code != http.StatusCreated {
		t.Fatalf("status %d: %v", code, out)
	}
	want := app.JoinRequest{Code: "k7qm-4xwd", Serial: "PF3ABCDE", Vendor: "LENOVO", Model: "21BR", CPU: "GenuineIntel", Virtualisation: "none"}
	if f.got != want {
		t.Fatalf("the service was asked %+v", f.got)
	}
	console, _ := out["console"].(map[string]any)
	if out["tag"] != "laptop-3abcde" || out["credential"] != "sxt_device-laptop-3abcde_x" ||
		console["url"] != "https://console.example.test" || console["branch"] != "stable" {
		t.Fatalf("answer %v", out)
	}
	if !strings.Contains(f.logs.String(), "laptop-3abcde") || strings.Contains(f.logs.String(), "sxt_") {
		t.Fatalf("log should name the device and never the credential:\n%s", f.logs.String())
	}
}

func TestJoinRefusalsInWords(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		status int
		says   string
		logs   string
	}{
		"unusable code": {app.ErrCodeRefused, http.StatusForbidden, "mistyped, used up or expired", "192.0.2.7"},
		"refusal":       {&app.JoinRefusal{Reason: "this laptop is already enrolled as lt-1"}, http.StatusConflict, "already enrolled as lt-1", "192.0.2.7"},
		"gate":          {&ports.ValidationError{Detail: "lt-9: attribute missing"}, http.StatusUnprocessableEntity, "attribute missing", "attribute missing"},
		"other":         {errors.New("disk full"), http.StatusInternalServerError, "could not finish the join", "disk full"},
	} {
		f := &fakeJoiner{err: tc.err}
		code, out := postJoin(t, joinServer(f), `{"code":"K7QM4XWD"}`)
		msg, _ := out["error"].(string)
		if code != tc.status || !strings.Contains(msg, tc.says) {
			t.Errorf("%s: %d %q", name, code, msg)
		}
		if _, leaked := out["credential"]; leaked {
			t.Errorf("%s: a refusal carries a credential", name)
		}
		if !strings.Contains(f.logs.String(), tc.logs) {
			t.Errorf("%s: log lacks %q:\n%s", name, tc.logs, f.logs.String())
		}
		if name == "other" && strings.Contains(msg, "disk full") {
			t.Errorf("an internal error reached the laptop: %q", msg)
		}
	}
}

func TestJoinRefusesWhatIsNotARequest(t *testing.T) {
	f := &fakeJoiner{}
	for _, body := range []string{"", "{", `{"code": 7}`, `{"code":"` + strings.Repeat("A", 20<<10) + `"}`} {
		if code, _ := postJoin(t, joinServer(f), body); code != http.StatusBadRequest {
			t.Errorf("body %.20q: %d", body, code)
		}
	}
	if f.got != (app.JoinRequest{}) {
		t.Fatal("a malformed request reached the service")
	}
	rec := httptest.NewRecorder()
	joinServer(f).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/join", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/join = %d", rec.Code)
	}
}

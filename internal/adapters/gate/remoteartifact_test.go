package gate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.overheid.nl/MinBZK/DAWO-Sextant/internal/ports"
)

// The console asks the runner for the installer with its token, reads the
// phase and file, says the runner's own reason when it refuses, and passes
// the image through untouched.
func TestRemoteArtifact(t *testing.T) {
	var asked artifactRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"phase":"failed","detail":"unauthorized"}`)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/artifact":
			_ = json.NewDecoder(r.Body).Decode(&asked)
			if asked.Rev == "nocache" {
				w.WriteHeader(http.StatusNotImplemented)
				_, _ = io.WriteString(w, `{"phase":"failed","detail":"no cache directory on this runner"}`)
				return
			}
			_, _ = io.WriteString(w, `{"phase":"done","file":"x.iso","size":8}`)
		case r.Method == http.MethodGet && r.URL.Path == "/artifact/abc1234/installer":
			_, _ = io.WriteString(w, "an image")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	ctx := context.Background()

	b := NewRemoteBuilder(srv.URL, "tok")
	st, err := b.EnsureArtifact(ctx, "abc1234", "installer")
	if err != nil || st.Phase != ports.BuildDone || st.File != "x.iso" || st.Size != 8 {
		t.Fatalf("ensure = %+v, %v", st, err)
	}
	if asked.Rev != "abc1234" || asked.Name != "installer" {
		t.Errorf("runner was asked %+v", asked)
	}
	if _, err := b.EnsureArtifact(ctx, "nocache", "installer"); err == nil || !strings.Contains(err.Error(), "no cache directory") {
		t.Errorf("refusal = %v, want the runner's reason", err)
	}
	if _, err := NewRemoteBuilder(srv.URL, "wrong").EnsureArtifact(ctx, "abc1234", "installer"); err == nil {
		t.Error("a wrong token was not an error")
	}

	rc, err := b.OpenArtifact(ctx, "abc1234", "installer")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "an image" {
		t.Errorf("image = %q", got)
	}
	if _, err := b.OpenArtifact(ctx, "def5678", "installer"); err == nil {
		t.Error("a missing image was not an error")
	}
}

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newArtifactServer is a runner with a cache directory whose artifact
// builds write a fake image instead of running nix.
func newArtifactServer(t *testing.T, build func(ctx context.Context, dir, attr, link string) error) *server {
	t.Helper()
	s, _ := newBuildServer(t, nil)
	s.cacheDir = filepath.Join(t.TempDir(), "cache")
	s.artifactFn = build
	return s
}

// fakeImage builds what nix build leaves: an out-link holding iso/<name>.iso.
func fakeImage(builds *atomic.Int32) func(context.Context, string, string, string) error {
	return func(_ context.Context, _, attr, link string) error {
		builds.Add(1)
		if attr != "packages.x86_64-linux.installer" {
			return errors.New("wrong attribute " + attr)
		}
		if err := os.MkdirAll(filepath.Join(link, "iso"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(link, "iso", "sextant-installer.iso"), []byte("an image"), 0o644)
	}
}

func postArtifact(t *testing.T, s *server, token, body string) (int, artifactResponse) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/artifact", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.handleArtifact(rec, req)
	var ar artifactResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &ar)
	return rec.Code, ar
}

func waitArtifact(t *testing.T, s *server, body, want string) artifactResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, ar := postArtifact(t, s, "sesame", body)
		if ar.Phase == want {
			return ar
		}
		if time.Now().After(deadline) {
			t.Fatalf("phase %q (%s), want %q", ar.Phase, ar.Detail, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func getArtifact(s *server, token, rev string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/artifact/"+rev+"/installer", nil)
	req.SetPathValue("rev", rev)
	req.SetPathValue("name", "installer")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	s.handleArtifactFile(rec, req)
	return rec
}

func TestArtifactRequiresAuthAKnownNameAndARevision(t *testing.T) {
	var builds atomic.Int32
	s := newArtifactServer(t, fakeImage(&builds))
	for _, c := range []struct {
		token, body string
		code        int
	}{
		{"", `{"rev":"abc1234","name":"installer"}`, http.StatusUnauthorized},
		{"sesame", `not json`, http.StatusBadRequest},
		{"sesame", `{"rev":"-x","name":"installer"}`, http.StatusBadRequest},
		{"sesame", `{"rev":"abc1234","name":"../../etc"}`, http.StatusBadRequest},
	} {
		if code, _ := postArtifact(t, s, c.token, c.body); code != c.code {
			t.Errorf("%q with %q = %d, want %d", c.body, c.token, code, c.code)
		}
	}
	if builds.Load() != 0 {
		t.Error("a refused request started a build")
	}
	s.cacheDir = ""
	if code, _ := postArtifact(t, s, "sesame", `{"rev":"abc1234","name":"installer"}`); code != http.StatusNotImplemented {
		t.Errorf("without a cache directory = %d, want 501", code)
	}
}

// One build per (rev, name), the file is handed to the token holder only,
// and a restarted runner reports a file it already has as done.
func TestArtifactIsBuiltOnceAndHandedOver(t *testing.T) {
	var builds atomic.Int32
	s := newArtifactServer(t, fakeImage(&builds))
	body := `{"rev":"abc1234","name":"installer"}`
	if _, ar := postArtifact(t, s, "sesame", body); ar.Phase != "building" {
		t.Fatalf("first call = %q", ar.Phase)
	}
	ar := waitArtifact(t, s, body, "done")
	if ar.File != "sextant-installer.iso" || ar.Size != int64(len("an image")) {
		t.Errorf("done = %+v", ar)
	}
	postArtifact(t, s, "sesame", body)
	if n := builds.Load(); n != 1 {
		t.Errorf("%d builds for one revision", n)
	}

	rec := getArtifact(s, "sesame", "abc1234")
	if rec.Code != 200 || rec.Body.String() != "an image" ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "sextant-installer.iso") {
		t.Errorf("download = %d %q %q", rec.Code, rec.Body.String(), rec.Header().Get("Content-Disposition"))
	}
	if rec := getArtifact(s, "", "abc1234"); rec.Code != http.StatusUnauthorized {
		t.Errorf("download without the token = %d", rec.Code)
	}
	if rec := getArtifact(s, "sesame", "def5678"); rec.Code != http.StatusNotFound {
		t.Errorf("download of a revision never built = %d", rec.Code)
	}

	restarted := newArtifactServer(t, fakeImage(&builds))
	restarted.cacheDir = s.cacheDir
	if _, ar := postArtifact(t, restarted, "sesame", body); ar.Phase != "done" || builds.Load() != 1 {
		t.Errorf("after a restart = %+v, builds %d", ar, builds.Load())
	}
}

// Only the newest two revisions stay; an older one is gone and asking for it
// again builds it again.
func TestArtifactKeepsTwoRevisions(t *testing.T) {
	var builds atomic.Int32
	s := newArtifactServer(t, fakeImage(&builds))
	revs := []string{"aaa1111", "bbb2222", "ccc3333"}
	for i, rev := range revs {
		body := `{"rev":"` + rev + `","name":"installer"}`
		postArtifact(t, s, "sesame", body)
		waitArtifact(t, s, body, "done")
		// Distinct times, so "newest" means something on a fast disk.
		at := time.Now().Add(time.Duration(i-10) * time.Minute)
		_ = os.Chtimes(filepath.Join(s.artifactDir(), rev+"-installer"), at, at)
	}
	// The prune ran after the third build, with times set after it: run it
	// again on the times as set.
	s.buildMu.Lock()
	s.pruneArtifacts("installer")
	s.buildMu.Unlock()
	if rec := getArtifact(s, "sesame", "aaa1111"); rec.Code != http.StatusNotFound {
		t.Errorf("the oldest revision is still served: %d", rec.Code)
	}
	for _, rev := range revs[1:] {
		if rec := getArtifact(s, "sesame", rev); rec.Code != 200 {
			t.Errorf("%s = %d, want it kept", rev, rec.Code)
		}
	}
	body := `{"rev":"aaa1111","name":"installer"}`
	if _, ar := postArtifact(t, s, "sesame", body); ar.Phase != "building" {
		t.Errorf("a pruned revision = %q, want a new build", ar.Phase)
	}
	waitArtifact(t, s, body, "done")
}

// A failed build says why, and a build that leaves no image is a failure.
func TestArtifactFailureIsReported(t *testing.T) {
	s := newArtifactServer(t, func(context.Context, string, string, string) error {
		return errors.New("error: flake has no output installer")
	})
	body := `{"rev":"abc1234","name":"installer"}`
	postArtifact(t, s, "sesame", body)
	if ar := waitArtifact(t, s, body, "failed"); !strings.Contains(ar.Detail, "no output installer") {
		t.Errorf("detail %q", ar.Detail)
	}

	empty := newArtifactServer(t, func(context.Context, string, string, string) error { return nil })
	postArtifact(t, empty, "sesame", body)
	if ar := waitArtifact(t, empty, body, "failed"); !strings.Contains(ar.Detail, "0 images") {
		t.Errorf("detail %q", ar.Detail)
	}
}

// The image is never under the cache every device may read.
func TestArtifactsAreNotInTheCache(t *testing.T) {
	s := &server{cacheDir: "/var/lib/gate/cache"}
	if got := s.artifactDir(); strings.HasPrefix(got, s.cacheDir) {
		t.Errorf("artifacts at %s, inside the cache", got)
	}
}

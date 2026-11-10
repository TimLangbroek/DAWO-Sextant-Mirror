package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitIn runs git with no credential helper and no way to ask, so a refusal
// fails the command instead of waiting for a password. GIT_TERMINAL_PROMPT
// alone is not enough: with SSH_ASKPASS set (a desktop session) git asks
// through a window instead, and the test hangs on it.
func gitIn(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "credential.helper=", "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "SSH_ASKPASS=") && !strings.HasPrefix(kv, "GIT_ASKPASS=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// servedRepo is a bare repository with one commit on main holding fleet.json.
func servedRepo(t *testing.T) (dir, head string) {
	t.Helper()
	work := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"commit", "-q", "--allow-empty", "-m", "fleet"},
	} {
		if out, err := gitIn(work, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	head, _ = gitIn(work, "rev-parse", "HEAD")
	dir = filepath.Join(t.TempDir(), "served.git")
	if out, err := gitIn(work, "clone", "-q", "--bare", work, dir); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return dir, head
}

func TestFleetRepoServesEnrolledDevicesReadOnly(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("needs git") // the nix build sandbox has none; `just test` runs it
	}
	dir, head := servedRepo(t)
	devs := &fakeDevAuth{creds: map[string]string{"cred-1": "lt-1", "cred-2": "lt-2"}}
	retired := map[string]bool{"lt-2": true}
	var servedErr error
	mux := http.NewServeMux()
	NewFleetRepo(devs, func(context.Context) (string, error) { return dir, servedErr }).
		WithLifecycle(func(tag string) bool { return retired[tag] }).
		Routes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	at := func(user string) string {
		return strings.Replace(srv.URL, "http://", "http://"+user+"@", 1) + "/fleet.git"
	}

	// The device's own tag and credential: a clone, at the served revision.
	clone := filepath.Join(t.TempDir(), "clone")
	if out, err := gitIn(t.TempDir(), "clone", "-q", at("lt-1:cred-1"), clone); err != nil {
		t.Fatalf("an enrolled device cannot clone: %v\n%s", err, out)
	}
	if got, _ := gitIn(clone, "rev-parse", "HEAD"); got != head {
		t.Fatalf("cloned %s, served %s", got, head)
	}

	// Refused: no credential, a wrong one, another device's, a retired tag.
	for name, user := range map[string]string{
		"anonymous":        "",
		"wrong credential": "lt-1:nope",
		"another's":        "lt-1:cred-2",
		"retired":          "lt-2:cred-2",
	} {
		u := srv.URL + "/fleet.git"
		if user != "" {
			u = at(user)
		}
		if out, err := gitIn(t.TempDir(), "ls-remote", u); err == nil {
			t.Errorf("%s: fetch allowed:\n%s", name, out)
		}
	}

	// A push is refused before git sees it.
	if out, err := gitIn(clone, "commit", "-q", "--allow-empty", "-m", "from a device"); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if out, err := gitIn(clone, "push", "-q", at("lt-1:cred-1"), "HEAD:main"); err == nil {
		t.Fatalf("a device pushed to the fleet repository:\n%s", out)
	}
	if out, _ := gitIn(dir, "rev-parse", "main"); out != head {
		t.Fatalf("served main moved to %s", out)
	}
	for path, want := range map[string]int{
		"/fleet.git/info/refs?service=git-receive-pack": http.StatusForbidden,
		"/fleet.git/info/refs":                          http.StatusForbidden,
		"/fleet.git/HEAD":                               http.StatusNotFound,
	} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.SetBasicAuth("lt-1", "cred-1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("GET %s = %d, want %d", path, resp.StatusCode, want)
		}
	}

	// A copy that cannot be prepared is a 503, not a half-served fetch.
	servedErr = errors.New("disk full")
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/fleet.git/info/refs?service=git-upload-pack", nil)
	req.SetBasicAuth("lt-1", "cred-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("broken copy = %d, want 503", resp.StatusCode)
	}
}

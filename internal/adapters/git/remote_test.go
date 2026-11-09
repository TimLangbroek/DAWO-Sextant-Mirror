package git

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

func TestCheckRemoteURL(t *testing.T) {
	ok := []string{"https://git.example.com/org/fleet.git", "file:///srv/fleet.git", "http://127.0.0.1:3000/org/fleet.git", "http://localhost/fleet.git"}
	bad := []string{"http://git.example.com/fleet.git", "ssh://git@git.example.com/fleet.git", "git@git.example.com:org/fleet.git",
		"https://user:tok@git.example.com/fleet.git", "fleet", ""}
	for _, u := range ok {
		if _, err := CheckRemoteURL(u); err != nil {
			t.Errorf("%q refused: %v", u, err)
		}
	}
	for _, u := range bad {
		if _, err := CheckRemoteURL(u); err == nil {
			t.Errorf("%q accepted", u)
		}
	}
}

// bareRepo returns a file:// URL of a bare repository with one commit.
func bareRepo(t *testing.T) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "src")
	if err := Seed(context.Background(), src, fstest.MapFS{"fleet.json": {Data: []byte("{}")}}, "seed"); err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(t.TempDir(), "fleet.git")
	if out, err := exec.Command("git", "clone", "-q", "--bare", src, bare).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return "file://" + bare
}

func TestCloneAndLsRemote(t *testing.T) {
	ctx := context.Background()
	remote := bareRepo(t)
	if err := LsRemote(ctx, remote, "", ""); err != nil {
		t.Fatalf("ls-remote of a good repository: %v", err)
	}
	if err := LsRemote(ctx, "file:///nonexistent/fleet.git", "", ""); err == nil {
		t.Fatal("ls-remote of a missing repository succeeded")
	}
	dir := filepath.Join(t.TempDir(), "data", "overlay")
	if err := Clone(ctx, remote, "", "", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir, "origin"); err != nil {
		t.Fatal(err)
	}
	if out, _ := exec.Command("git", "-C", dir, "remote", "get-url", "origin").Output(); strings.TrimSpace(string(out)) != remote {
		t.Fatalf("origin is %q, want %q", out, remote)
	}
	if err := Clone(ctx, remote, "", "", dir); err == nil {
		t.Fatal("cloned over an existing directory")
	}
	failed := filepath.Join(t.TempDir(), "overlay")
	if err := Clone(ctx, "file:///nonexistent/fleet.git", "", "", failed); err == nil {
		t.Fatal("clone of a missing repository succeeded")
	}
	if _, err := os.Stat(failed); !os.IsNotExist(err) {
		t.Fatal("a failed clone left a directory behind")
	}
}

// The token reaches the server as Basic auth, and the error the operator
// sees does not repeat it.
func TestTokenGoesInTheHeaderOnly(t *testing.T) {
	var mu sync.Mutex
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if a := r.Header.Get("Authorization"); a != "" {
			got = a
		}
		mu.Unlock()
		http.Error(w, "no", http.StatusForbidden)
	}))
	defer srv.Close()

	const token = "tok-0123456789"
	err := LsRemote(context.Background(), srv.URL+"/org/fleet.git", "alice", token)
	if err == nil {
		t.Fatal("ls-remote against a refusing server succeeded")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("token in the error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:"+token)); got != want {
		t.Fatalf("Authorization %q, want %q", got, want)
	}
}

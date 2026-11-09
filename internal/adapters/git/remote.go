package git

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CheckRemoteURL refuses a repository address setup cannot use. HTTPS, or
// file:// for a repository on the same machine. Plain http only to a
// loopback host, because the token travels in the request and anything else
// would hand it to every hop on the way. SSH is not offered: it needs a key
// on this machine, which is a different setup from a token in a form.
func CheckRemoteURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" {
		return nil, fmt.Errorf("%q is not a repository address; use https://host/org/repo.git", raw)
	}
	switch u.Scheme {
	case "https", "file":
	case "http":
		host := u.Hostname()
		if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, fmt.Errorf("http:// would send the token unencrypted; use https://")
		}
	default:
		return nil, fmt.Errorf("%s:// is not supported here; use https://", u.Scheme)
	}
	if u.User != nil {
		return nil, fmt.Errorf("put the account and the token in their own fields, not in the address")
	}
	return u, nil
}

// LsRemote checks that the repository answers and the credential is good,
// without fetching anything.
func LsRemote(ctx context.Context, rawURL, user, token string) error {
	u, err := CheckRemoteURL(rawURL)
	if err != nil {
		return err
	}
	return runRemote(ctx, u, user, token, "ls-remote", "--heads", u.String())
}

// Clone clones the repository into dir. The credential is used for this one
// command and is not written anywhere: the clone's remote is the plain
// address, and the console's own forge identity (ADR 0022) supplies the
// credential from then on.
func Clone(ctx context.Context, rawURL, user, token, dir string) error {
	u, err := CheckRemoteURL(rawURL)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s already exists", dir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return err
	}
	if err := runRemote(ctx, u, user, token, "clone", "-q", "--", u.String(), dir); err != nil {
		_ = os.RemoveAll(dir) // a half-cloned directory would read as a repository
		return err
	}
	return nil
}

// runRemote runs one git command against a remote with an optional token.
// The token travels in an extra header set through git's environment
// configuration: not in the URL, so it never lands in .git/config or in
// git's error text, and not in the arguments, which every user on the
// machine can read in a process listing. A process's environment is
// readable only by its owner.
func runRemote(ctx context.Context, u *url.URL, user, token string, args ...string) error {
	argv := append([]string{"-c", "credential.helper=", "-c", "core.askPass=true"}, args...)
	// Never prompt: a missing or wrong token must fail, not hang the request
	// waiting for a terminal that is not there.
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if token != "" && u.Scheme != "file" {
		if user == "" {
			user = "sextant"
		}
		basic := base64.StdEncoding.EncodeToString([]byte(user + ":" + token))
		env = append(env, "GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.extraHeader",
			"GIT_CONFIG_VALUE_0=Authorization: Basic "+basic)
	}
	cctx, cancel := context.WithTimeout(ctx, netTimeout)
	defer cancel()
	// #nosec G204 - fixed "git" binary, argv slice (no shell); the URL was parsed and its scheme checked.
	cmd := exec.CommandContext(cctx, "git", argv...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s", args[0], firstLine(string(out)))
	}
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

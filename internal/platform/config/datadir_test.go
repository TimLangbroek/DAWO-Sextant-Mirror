package config

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testSessionKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))

func writeData(t *testing.T, dir, name, body string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

// A console started with nothing but a data directory written by setup can
// sign people in: every OIDC value, the secrets and the redirect URL come
// from the two files.
func TestDataDirAloneConfiguresSignIn(t *testing.T) {
	dir := t.TempDir()
	writeData(t, dir, SettingsFile, `{"publicUrl":"https://console.example.com/","oidcIssuer":"https://id.example.com",
		"oidcClientId":"sextant","ownerSubjects":["sub-1"],"repoDir":"/data/overlay"}`, 0o600)
	writeData(t, dir, KeysFile, `{"sessionKey":"`+testSessionKey+`","oidcClientSecret":"s3cret",
		"secretKey":"sk","checkinToken":"ct"}`, 0o600)

	c, err := Load([]string{"--data-dir", dir}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.OIDCIssuer != "https://id.example.com" || c.OIDCClientID != "sextant" || c.OIDCClientSecret != "s3cret" {
		t.Fatalf("oidc not filled: %q %q %q", c.OIDCIssuer, c.OIDCClientID, c.OIDCClientSecret)
	}
	if c.OIDCRedirectURL != "https://console.example.com/callback" || c.ConsoleURL != "https://console.example.com" {
		t.Fatalf("urls not derived from publicUrl: %q %q", c.OIDCRedirectURL, c.ConsoleURL)
	}
	if len(c.SessionKey) != 32 || c.SecretKey != "sk" || c.CheckinToken != "ct" || c.RepoDir != "/data/overlay" {
		t.Fatalf("keys or repo not filled: %d %q %q %q", len(c.SessionKey), c.SecretKey, c.CheckinToken, c.RepoDir)
	}
	if len(c.OwnerSubjects) != 1 || c.OwnerSubjects[0] != "sub-1" {
		t.Fatalf("owner subjects: %v", c.OwnerSubjects)
	}
	if len(c.Deployment) != 0 {
		t.Fatalf("nothing came from the deployment, got %v", c.Deployment)
	}
}

// The deployment always wins, and setup is told which values it fixed.
func TestDeploymentWinsOverDataDir(t *testing.T) {
	dir := t.TempDir()
	writeData(t, dir, SettingsFile, `{"oidcIssuer":"https://file.example.com","oidcClientId":"from-file","repoDir":"/file"}`, 0o600)
	writeData(t, dir, KeysFile, `{"oidcClientSecret":"from-file","sessionKey":"`+testSessionKey+`"}`, 0o600)

	c, err := Load([]string{"--data-dir", dir, "--oidc-client-id", "from-flag", "--oidc-redirect-url", "https://x/callback"},
		env(map[string]string{"SEXTANT_REPO": "/env", "SEXTANT_OIDC_CLIENT_SECRET": "from-env"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.OIDCClientID != "from-flag" || c.RepoDir != "/env" || c.OIDCClientSecret != "from-env" {
		t.Fatalf("deployment lost: %q %q %q", c.OIDCClientID, c.RepoDir, c.OIDCClientSecret)
	}
	if c.OIDCIssuer != "https://file.example.com" {
		t.Fatalf("unset value not filled from file: %q", c.OIDCIssuer)
	}
	for _, k := range []string{"OIDC_CLIENT_ID", "REPO", "OIDC_CLIENT_SECRET"} {
		if !c.Deployment[k] {
			t.Errorf("Deployment[%s] not recorded: %v", k, c.Deployment)
		}
	}
	if c.Deployment["OIDC_ISSUER"] {
		t.Errorf("OIDC_ISSUER came from the file, not the deployment")
	}
}

// An explicit redirect URL is not overwritten by the one publicUrl implies.
func TestPublicURLDoesNotOverrideExplicitRedirect(t *testing.T) {
	c, err := Load([]string{"--public-url", "https://console.example.com", "--oidc-redirect-url", "https://other.example.com/cb"}, env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.OIDCRedirectURL != "https://other.example.com/cb" || c.ConsoleURL != "https://console.example.com" {
		t.Fatalf("got redirect %q console %q", c.OIDCRedirectURL, c.ConsoleURL)
	}
}

func TestDataDirRefusals(t *testing.T) {
	cases := []struct {
		name, file, body string
		mode             os.FileMode
		want             string
	}{
		{"keys readable by others", KeysFile, `{"secretKey":"x"}`, 0o644, "chmod 600"},
		{"unknown field in settings", SettingsFile, `{"oidcIssuer":"x","oidcIsuer":"typo"}`, 0o600, "unknown field"},
		{"session key of the wrong length", KeysFile, `{"sessionKey":"` + base64.StdEncoding.EncodeToString([]byte("short")) + `"}`, 0o600, "exactly 32 bytes"},
		{"not json", SettingsFile, `oidcIssuer = "x"`, 0o600, SettingsFile},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeData(t, dir, tc.file, tc.body, tc.mode)
			_, err := Load([]string{"--data-dir", dir}, env(nil))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// A data directory that does not exist yet, or holds nothing, changes
// nothing: that is every deployment that predates setup.
func TestEmptyOrMissingDataDirChangesNothing(t *testing.T) {
	for _, dir := range []string{"", t.TempDir(), filepath.Join(t.TempDir(), "not-there")} {
		c, err := Load([]string{"--data-dir", dir}, env(nil))
		if err != nil {
			t.Fatalf("%q: %v", dir, err)
		}
		if c.OIDCIssuer != "" || c.RepoDir != "" || c.SessionKey != nil || c.ConsoleURL != "" {
			t.Fatalf("%q: something was filled: %+v", dir, c)
		}
	}
}

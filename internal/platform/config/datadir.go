package config

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The data directory holds what first-start setup asked for (design 0013).
// It sits BELOW flags and environment: a value the deployment sets always
// wins, and the files only fill what nobody set. So Helm values and NixOS
// options stay the authority, and a console started with nothing configured
// can still keep the answers it was given.
const (
	SettingsFile = "settings.json"
	KeysFile     = "keys.json"
)

// Settings is the non-secret half of the data directory.
type Settings struct {
	PublicURL       string   `json:"publicUrl,omitempty"`
	RepoDir         string   `json:"repoDir,omitempty"`
	GitRemote       string   `json:"gitRemote,omitempty"`
	OIDCIssuer      string   `json:"oidcIssuer,omitempty"`
	OIDCClientID    string   `json:"oidcClientId,omitempty"`
	OIDCGroupsClaim string   `json:"oidcGroupsClaim,omitempty"`
	OrgName         string   `json:"orgName,omitempty"`
	OwnerSubjects   []string `json:"ownerSubjects,omitempty"`
	// ForgeHost and ForgeUser go with Keys.ForgeToken: the credential setup
	// cloned an existing repository with, waiting to be handed to the
	// console's own forge identity on the next start.
	ForgeHost string `json:"forgeHost,omitempty"`
	ForgeUser string `json:"forgeUser,omitempty"`
}

// Keys is the secret half. Base64 where the environment variable is base64,
// so a value can be moved between keys.json and the environment unchanged.
type Keys struct {
	SessionKey       string `json:"sessionKey,omitempty"`
	SecretKey        string `json:"secretKey,omitempty"`
	CheckinToken     string `json:"checkinToken,omitempty"`
	OIDCClientSecret string `json:"oidcClientSecret,omitempty"`
	// ForgeToken lives here only until the next start moves it into the
	// database, sealed, where the console can rotate it (ADR 0022).
	ForgeToken string `json:"forgeToken,omitempty"`
}

// ReadDataDir reads both files. A missing file is an empty half, not an
// error: a fresh console has neither. Anything else wrong with a file that
// exists is an error, because a setting that silently does not apply is the
// failure this project keeps paying for.
func ReadDataDir(dir string) (Settings, Keys, error) {
	var s Settings
	var k Keys
	if dir == "" {
		return s, k, nil
	}
	if err := readStrict(filepath.Join(dir, SettingsFile), &s, false); err != nil {
		return s, k, err
	}
	if err := readStrict(filepath.Join(dir, KeysFile), &k, true); err != nil {
		return s, k, err
	}
	return s, k, nil
}

func readStrict(path string, dst any, secret bool) error {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	// Same rule ssh applies to a private key: a secret file others can read
	// is refused, not used. Refusing says what to do; using it would leave
	// the console running on keys that are no longer secret.
	if secret && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s is readable by other users (mode %o); run: chmod 600 %s", path, info.Mode().Perm(), path)
	}
	raw, err := os.ReadFile(path) // #nosec G304 - the operator's own data directory
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	// An unknown field is a typo or a file from a newer version. Either way
	// the setting the operator meant does not apply, so say so.
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// applyDataDir fills every field the deployment left unset from the data
// directory, and records which fields the deployment did set so setup can
// show them as fixed. setByFlag names the flags given on the command line.
func (c *Config) applyDataDir(getenv Getenv, setByFlag map[string]bool) error {
	s, k, err := ReadDataDir(c.DataDir)
	if err != nil {
		return err
	}
	c.Deployment = map[string]bool{}
	// Keyed by the environment name without the prefix, which every setting
	// has; not every setting has a flag.
	set := func(flagName, envKey string) bool {
		if (flagName != "" && setByFlag[flagName]) || getenv(EnvPrefix+envKey) != "" {
			c.Deployment[envKey] = true
			return true
		}
		return false
	}
	fill := func(dst *string, flagName, envKey, val string) {
		if !set(flagName, envKey) && val != "" {
			*dst = val
		}
	}
	fill(&c.PublicURL, "public-url", "PUBLIC_URL", s.PublicURL)
	fill(&c.RepoDir, "repo", "REPO", s.RepoDir)
	fill(&c.GitRemote, "git-remote", "GIT_REMOTE", s.GitRemote)
	fill(&c.OIDCIssuer, "oidc-issuer", "OIDC_ISSUER", s.OIDCIssuer)
	fill(&c.OIDCClientID, "oidc-client-id", "OIDC_CLIENT_ID", s.OIDCClientID)
	fill(&c.OIDCGroupsClaim, "oidc-groups-claim", "OIDC_GROUPS_CLAIM", s.OIDCGroupsClaim)
	fill(&c.OrgName, "", "ORG_NAME", s.OrgName)
	if !set("owner-subjects", "OWNER_SUBJECTS") && len(s.OwnerSubjects) > 0 {
		c.OwnerSubjects = s.OwnerSubjects
	}
	// Not settings anyone sets from the deployment: a forge credential the
	// deployment manages arrives as a mounted netrc, as it always did.
	if k.ForgeToken != "" {
		c.SetupForge = &SetupForge{Host: s.ForgeHost, User: s.ForgeUser, Token: k.ForgeToken}
	}

	// Secrets: environment or keys.json, never a flag.
	fill(&c.SecretKey, "", "SECRET_KEY", k.SecretKey)
	fill(&c.CheckinToken, "", "CHECKIN_TOKEN", k.CheckinToken)
	fill(&c.OIDCClientSecret, "", "OIDC_CLIENT_SECRET", k.OIDCClientSecret)
	if !set("", "SESSION_KEY") && k.SessionKey != "" {
		key, err := decodeSessionKey(k.SessionKey)
		if err != nil {
			return fmt.Errorf("%s: sessionKey: %w", filepath.Join(c.DataDir, KeysFile), err)
		}
		c.SessionKey = key
	}

	// One public address, from which the two URLs that used to be set
	// separately follow. An explicit value for either still wins.
	if c.PublicURL != "" {
		base := strings.TrimRight(c.PublicURL, "/")
		if c.OIDCRedirectURL == "" {
			c.OIDCRedirectURL = base + "/callback"
		}
		if c.ConsoleURL == "" {
			c.ConsoleURL = base
		}
	}
	// An https address means TLS sits in front, so cookies can be Secure
	// and should be. Without this, the address setup writes would leave the
	// next start refusing sign-in on any listener but loopback.
	if !set("secure-cookies", "SECURE_COOKIES") && strings.HasPrefix(c.PublicURL, "https://") {
		c.SecureCookies = true
	}
	return nil
}

func decodeSessionKey(v string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return nil, fmt.Errorf("not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("must decode to exactly 32 bytes, got %d", len(key))
	}
	return key, nil
}

// DropForgeToken removes the forge token from keys.json once the console
// holds it sealed. The rest of the file stays as it was.
func DropForgeToken(dir string) error {
	_, k, err := ReadDataDir(dir)
	if err != nil {
		return err
	}
	if k.ForgeToken == "" {
		return nil
	}
	k.ForgeToken = ""
	return WriteKeys(dir, k)
}

// WriteSettings and WriteKeys replace one file of the data directory. Both
// are written 0600 into a 0700 directory, through a temporary file and a
// rename, so a crash leaves the old file or the new one and never half of
// either: a half-written keys.json would fail the next start, and the
// refusal would read as corruption of something the operator never touched.
func WriteSettings(dir string, s Settings) error { return writeJSON(dir, SettingsFile, s) }

// WriteKeys writes keys.json; see WriteSettings.
func WriteKeys(dir string, k Keys) error { return writeJSON(dir, KeysFile, k) }

func writeJSON(dir, name string, v any) error {
	if dir == "" {
		return errors.New("no data directory configured (--data-dir)")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+name+"-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // a no-op once renamed
	// CreateTemp already opens 0600; set it anyway so the rule does not hang
	// on a library default.
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

// flagsSet returns the names of the flags given on the command line.
func flagsSet(fs *flag.FlagSet) map[string]bool {
	out := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { out[f.Name] = true })
	return out
}

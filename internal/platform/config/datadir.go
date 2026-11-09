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
}

// Keys is the secret half. Base64 where the environment variable is base64,
// so a value can be moved between keys.json and the environment unchanged.
type Keys struct {
	SessionKey       string `json:"sessionKey,omitempty"`
	SecretKey        string `json:"secretKey,omitempty"`
	CheckinToken     string `json:"checkinToken,omitempty"`
	OIDCClientSecret string `json:"oidcClientSecret,omitempty"`
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

// flagsSet returns the names of the flags given on the command line.
func flagsSet(fs *flag.FlagSet) map[string]bool {
	out := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { out[f.Name] = true })
	return out
}

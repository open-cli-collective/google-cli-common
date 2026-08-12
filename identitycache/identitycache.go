// Package identitycache caches the verified account email per credential
// profile, so `profiles list` can show WHICH account each profile holds
// without a per-profile API call. It is derived data (re-verifiable via the
// Gmail profile any time), so it lives in the OS cache dir — wiped by
// `config clear --all`, never authoritative, and deliberately not in the
// keyring: an email address is not an access secret (§1.12), and the §1.5.2
// bundle-key allowlist stays exactly [oauth_token].
package identitycache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/open-cli-collective/google-cli-common/config"
)

// FileName is the cache file, under the CLI's OS cache dir.
const FileName = "profile_identities.json"

// Entry is what we remember about one profile's account.
type Entry struct {
	// Email is the account address as last verified against the API.
	Email string `json:"email"`
	// VerifiedAt is when Email was last confirmed live (init or
	// `profiles list --check`) — shown so a stale cache reads as stale.
	VerifiedAt time.Time `json:"verified_at"`
}

// path resolves the cache file location, creating the cache dir.
func path() (string, error) {
	dir, err := config.GetCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, FileName), nil
}

// Load returns the cached profile→identity map. An absent or unreadable
// cache is an empty map, never an error worth failing a command over: the
// cache is a display nicety and callers always treat it as best-effort.
func Load() map[string]Entry {
	p, err := path()
	if err != nil {
		return map[string]Entry{}
	}
	data, err := os.ReadFile(p) //nolint:gosec // path from our own cache dir
	if err != nil {
		return map[string]Entry{}
	}
	var m map[string]Entry
	if err := json.Unmarshal(data, &m); err != nil || m == nil {
		return map[string]Entry{}
	}
	return m
}

// Put records (or refreshes) a profile's verified email using an atomic
// temp-file → rename write, mirroring config.SaveConfig. Load-modify-save is
// racy across concurrent processes in principle, but the value is a
// re-derivable cache: the loser of a race costs one future re-verification,
// nothing more.
func Put(profile, email string) error {
	if profile == "" || email == "" {
		return fmt.Errorf("identitycache: profile and email are required")
	}
	m := Load()
	m[profile] = Entry{Email: email, VerifiedAt: time.Now().UTC()}

	p, err := path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), FileName+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp identity cache: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing temp identity cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp identity cache: %w", err)
	}
	if err := os.Chmod(tmpPath, config.TokenPerm); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("setting identity cache mode: %w", err)
	}
	if err := os.Rename(tmpPath, p); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("finalizing identity cache: %w", err)
	}
	return nil
}

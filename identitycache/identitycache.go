// Package identitycache caches the verified account email per credential
// profile, so `profiles list` can show WHICH account each profile holds
// without a per-profile API call. It is derived data (re-verifiable via the
// Gmail profile any time), so it lives in the OS cache dir via the shared
// cli-common cache envelope — wiped by `config clear --all`, never
// authoritative, and deliberately not in the keyring: an email address is
// not an access secret (§1.12), and the §1.5.2 bundle-key allowlist stays
// exactly [oauth_token].
package identitycache

import (
	"fmt"
	"time"

	clicache "github.com/open-cli-collective/cli-common/cache"

	"github.com/open-cli-collective/google-cli-common/config"
)

const (
	// resourceName is the cli-common cache resource; on disk the file is
	// <cachedir>/<instanceKey>/profile_identities.json.
	resourceName = "profile_identities"
	// instanceKey matches the cache package's single-instance convention.
	instanceKey = "default"
	// ttl is nominal: identities never auto-expire (freshness classification
	// is not consulted on read). Staleness is a display concern carried by
	// each Entry's VerifiedAt; a wrong cache heals on the next verified init
	// or `profiles list --check`.
	ttl = "8760h"
)

// Entry is what we remember about one profile's account.
type Entry struct {
	// Email is the account address as last verified against the API.
	Email string `json:"email"`
	// VerifiedAt is when Email was last confirmed live (init or
	// `profiles list --check`) — shown so a stale cache reads as stale.
	VerifiedAt time.Time `json:"verified_at"`
}

func locator() (clicache.Locator, error) {
	dir, err := config.GetCacheDir()
	if err != nil {
		return clicache.Locator{}, err
	}
	return clicache.Locator{Root: dir, InstanceKey: instanceKey}, nil
}

// Load returns the cached profile→identity map. An absent, corrupt, or
// version-mismatched cache is an empty map, never an error worth failing a
// command over: the cache is a display nicety and callers always treat it as
// best-effort (envelope corruption heals on the next Put).
func Load() map[string]Entry {
	loc, err := locator()
	if err != nil {
		return map[string]Entry{}
	}
	env, err := clicache.ReadResource[map[string]Entry](loc, resourceName)
	if err != nil || env.Data == nil {
		return map[string]Entry{}
	}
	return env.Data
}

// Put records (or refreshes) a profile's verified email through the shared
// envelope's atomic write. Load-modify-save is racy across concurrent
// processes in principle, but the value is a re-derivable cache: the loser
// of a race costs one future re-verification, nothing more.
func Put(profile, email string) error {
	if profile == "" || email == "" {
		return fmt.Errorf("identitycache: profile and email are required")
	}
	m := Load()
	m[profile] = Entry{Email: email, VerifiedAt: time.Now().UTC()}

	loc, err := locator()
	if err != nil {
		return err
	}
	return clicache.WriteResource(loc, resourceName, ttl, m)
}

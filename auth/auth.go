// Package auth provides OAuth2 authentication and credential management for Google APIs.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"

	"github.com/open-cli-collective/google-cli-common/config"
	"github.com/open-cli-collective/google-cli-common/keychain"
)

// CheckScopesMigration compares the registered CLI's currently-required scopes
// (config.Scopes(), set by the CLI via config.Register) against the scopes a
// token was previously granted. It returns a non-empty, actionable message
// when the token is missing any now-required scope, so a CLI that has widened
// its scope set can prompt the user to re-authenticate. The scope set,
// descriptions, and product name all come from the registered identity, so the
// same logic serves any CLI backed by this library.
func CheckScopesMigration(grantedScopes []string) string {
	if len(grantedScopes) == 0 {
		return ""
	}

	granted := make(map[string]bool, len(grantedScopes))
	for _, s := range grantedScopes {
		granted[s] = true
	}

	var missing []string
	for _, required := range config.Scopes() {
		if !granted[required] {
			missing = append(missing, required)
		}
	}

	if len(missing) == 0 {
		return ""
	}

	descriptions := config.ScopeDescriptions()
	msg := fmt.Sprintf("This command requires additional permissions.\nYour token is missing one or more required scopes.\n\nRun '%s init' to re-authenticate with the updated scopes.\n\nNew scopes:\n", config.ProductName())
	for _, s := range missing {
		desc := descriptions[s]
		if desc == "" {
			desc = s
		}
		msg += "  - " + desc + "\n"
	}
	return msg
}

// GetOAuthConfig loads the OAuth client config from the deployment-material
// OAuth client JSON referenced by config.yml's oauth_client_path (§1.2 — not
// a secret; lives on disk, never the keyring), with all scopes.
func GetOAuthConfig() (*oauth2.Config, error) {
	cfg, err := config.LoadConfigForRuntime()
	if err != nil {
		return nil, err
	}
	path := config.ExpandPath(cfg.OAuthClientPath)
	b, err := os.ReadFile(path) //nolint:gosec // deployment-material path from config
	if err != nil {
		return nil, fmt.Errorf("unable to read OAuth client JSON %s (run '%s init'): %w",
			config.ShortenPath(path), config.ProductName(), err)
	}
	return google.ConfigFromJSON(b, config.Scopes()...)
}

// GetHTTPClient returns an HTTP client with OAuth2 authentication. The token
// is read solely from the OS keyring via credstore (§1.1/§2.3 — no
// security/secret-tool shell-out, no token.json fallback). The active
// credential_ref is captured once here; refreshed tokens persist back to that
// exact ref via the closure passed to the token source (the sole sanctioned
// non-ingress keyring write). Returns an actionable error if no token exists.
func GetHTTPClient(ctx context.Context) (*http.Client, error) {
	st, err := keychain.Open()
	if err != nil {
		return nil, err
	}
	return clientFromStore(ctx, st)
}

// GetHTTPClientForRef is GetHTTPClient bound to an explicit credential ref
// instead of the active one — the seam behind `profiles list --check`, which
// probes every stored profile's token, not just the active profile's. Opens
// via keychain.OpenRef, so the one-time migration never runs against a
// non-active ref.
func GetHTTPClientForRef(ctx context.Context, ref string) (*http.Client, error) {
	st, err := keychain.OpenRef(ref)
	if err != nil {
		return nil, err
	}
	return clientFromStore(ctx, st)
}

// clientFromStore builds the authenticated client from an already-open Store,
// closing it before returning (the client must not hold the Store for its
// lifetime). Shared by the active-ref and explicit-ref entry points so the
// persist-on-refresh and error-attribution behavior can't diverge.
func clientFromStore(ctx context.Context, st *keychain.Store) (*http.Client, error) {
	oauthCfg, err := GetOAuthConfig()
	if err != nil {
		_ = st.Close()
		return nil, err
	}

	tok, err := st.Token()
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("no OAuth token stored for credential %s (selected via %s) - run '%s init' first: %w",
			st.Ref(), keychain.DescribeRefSource(st.RefSource()), config.ProductName(), err)
	}
	ref := st.Ref()
	refSource := st.RefSource()
	_ = st.Close() // do not hold the Store for the client's lifetime

	persist := func(t *oauth2.Token) error {
		ps, perr := keychain.OpenRef(ref) // runMigration=false: refresh is not ingress
		if perr != nil {
			return perr
		}
		defer func() { _ = ps.Close() }()
		return ps.SetToken(t)
	}

	tokenSource := keychain.NewPersistentTokenSource(ctx, oauthCfg, tok, persist)
	return oauth2.NewClient(ctx, &attributedTokenSource{base: tokenSource, ref: ref, source: refSource}), nil
}

// attributedTokenSource decorates auth failures from the wrapped source with
// the credential ref that failed and where that ref was selected. Without
// this, an expired/revoked refresh token surfaces as a bare
// `oauth2: "invalid_grant" ...` — which reads as "the tool is broken" when
// the real state is "this one profile is stale and others may be fine".
// Non-auth errors (network, API outages) pass through untouched: naming the
// profile there would misattribute an infrastructure failure to a credential.
type attributedTokenSource struct {
	base   oauth2.TokenSource
	ref    string
	source config.RefSource
}

func (a *attributedTokenSource) Token() (*oauth2.Token, error) {
	tok, err := a.base.Token()
	if err != nil && IsAuthError(err) {
		prod := config.ProductName()
		return nil, fmt.Errorf("credential %s (selected via %s) can no longer authenticate: %w; other profiles may be unaffected - run '%s profiles list' to check them, or '%s init' to re-authenticate this one",
			a.ref, keychain.DescribeRefSource(a.source), err, prod, prod)
	}
	return tok, err
}

// IsAuthError reports whether err means the stored token no longer
// authenticates — i.e. re-auth is the fix — as opposed to a transient
// network/API failure. Shared by init's re-auth gate and the runtime
// error-attribution wrapper so "what counts as an auth failure" has exactly
// one definition.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	// An expired/revoked refresh token fails inside the oauth2 transport
	// (the request never reaches the API): the token endpoint returns HTTP
	// 400 with error code "invalid_grant", surfaced as *oauth2.RetrieveError.
	// Testing-mode OAuth apps expire their refresh tokens every 7 days, and
	// the resulting error carries no 401 for the fallback below.
	var retrieveErr *oauth2.RetrieveError
	if ok := errors.As(err, &retrieveErr); ok && retrieveErr.ErrorCode == "invalid_grant" {
		return true
	}
	var apiErr *googleapi.Error
	if ok := errors.As(err, &apiErr); ok {
		return apiErr.Code == http.StatusUnauthorized
	}
	// String fallback for wrapped/legacy error shapes. "invalid_grant" and
	// "Token has been expired or revoked" only ever come from the OAuth
	// token endpoint's error response, so they are safe to treat as
	// definitive without a status code; a bare 401 still needs corroboration.
	errStr := err.Error()
	if strings.Contains(errStr, "invalid_grant") ||
		strings.Contains(errStr, "Token has been expired or revoked") {
		return true
	}
	return strings.Contains(errStr, "401") && strings.Contains(errStr, "Invalid Credentials")
}

// GetAuthURL returns the OAuth authorization URL for the given config
func GetAuthURL(config *oauth2.Config) string {
	return config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
}

// ExchangeAuthCode exchanges an authorization code for a token
func ExchangeAuthCode(ctx context.Context, config *oauth2.Config, code string) (*oauth2.Token, error) {
	return config.Exchange(ctx, code)
}

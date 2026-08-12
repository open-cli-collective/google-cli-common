package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"github.com/open-cli-collective/google-cli-common/config"
)

// TestIsAuthError pins the auth-vs-transient classification. Moved from
// initcmd when the helper was promoted to this package so init's re-auth gate
// and the runtime error-attribution wrapper share one definition.
func TestIsAuthError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"generic error", errors.New("something went wrong"), false},
		{"network error", errors.New("connection refused"), false},
		{"googleapi 401", &googleapi.Error{Code: http.StatusUnauthorized, Message: "Invalid Credentials"}, true},
		{"googleapi 403", &googleapi.Error{Code: http.StatusForbidden, Message: "Access denied"}, false},
		{"googleapi 404", &googleapi.Error{Code: http.StatusNotFound, Message: "Not found"}, false},
		{"text 401 + Invalid Credentials", errors.New("googleapi: Error 401: Invalid Credentials"), true},
		{"text 401 + invalid_grant", errors.New("oauth2: 401 invalid_grant: Token has been expired"), true},
		{"text Token has been expired or revoked", errors.New("401: Token has been expired or revoked"), true},
		{"text 401 alone", errors.New("HTTP 401 response"), false},
		// An expired/revoked refresh token fails token *refresh* (HTTP 400
		// invalid_grant from the token endpoint), so it carries no 401 —
		// this is the shape a Testing-mode OAuth app produces every 7 days.
		{"RetrieveError invalid_grant", &oauth2.RetrieveError{
			Response:         &http.Response{StatusCode: http.StatusBadRequest},
			ErrorCode:        "invalid_grant",
			ErrorDescription: "Token has been expired or revoked.",
		}, true},
		{"RetrieveError invalid_grant wrapped like production", fmt.Errorf("getting profile: %w",
			&url.Error{Op: "Get", URL: "https://gmail.googleapis.com/gmail/v1/users/me/profile", Err: &oauth2.RetrieveError{
				Response:  &http.Response{StatusCode: http.StatusBadRequest},
				ErrorCode: "invalid_grant",
			}}), true},
		{"RetrieveError other code", &oauth2.RetrieveError{
			Response:  &http.Response{StatusCode: http.StatusServiceUnavailable},
			ErrorCode: "temporarily_unavailable",
		}, false},
		{"text invalid_grant without 401", errors.New(`oauth2: "invalid_grant" "Token has been expired or revoked."`), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsAuthError(tt.err); got != tt.expected {
				t.Errorf("IsAuthError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

// staticErrSource is a TokenSource that always fails with a fixed error.
type staticErrSource struct{ err error }

func (s staticErrSource) Token() (*oauth2.Token, error) { return nil, s.err }

// TestAttributedTokenSource_NamesRefOnAuthError is the regression test for
// the mis-diagnosis this wrapper exists to prevent: a bare
// `oauth2: "invalid_grant"` with no profile name read as "the tool is dead"
// when only the active profile's token was stale.
func TestAttributedTokenSource_NamesRefOnAuthError(t *testing.T) {
	base := staticErrSource{err: &oauth2.RetrieveError{
		Response:         &http.Response{StatusCode: http.StatusBadRequest},
		ErrorCode:        "invalid_grant",
		ErrorDescription: "Token has been expired or revoked.",
	}}
	ats := &attributedTokenSource{base: base, ref: "google-readonly/default", source: config.RefSourceConfig}

	_, err := ats.Token()
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, want := range []string{
		"credential google-readonly/default",
		"config.yml credential_ref",
		"invalid_grant",
		"profiles list",
		"init",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	// The original error must stay errors.As-able through the wrap so
	// callers classifying with IsAuthError (or inspecting RetrieveError)
	// still work.
	var retrieveErr *oauth2.RetrieveError
	if !errors.As(err, &retrieveErr) {
		t.Errorf("wrapped error lost the underlying *oauth2.RetrieveError")
	}
}

// TestAttributedTokenSource_PassesThroughNonAuthErrors proves a network-class
// failure is NOT attributed to the credential (that would misdiagnose an
// outage as a stale profile).
func TestAttributedTokenSource_PassesThroughNonAuthErrors(t *testing.T) {
	netErr := errors.New("dial tcp: connection refused")
	ats := &attributedTokenSource{base: staticErrSource{err: netErr}, ref: "google-readonly/default", source: config.RefSourceConfig}

	_, err := ats.Token()
	if !errors.Is(err, netErr) {
		t.Fatalf("err = %v, want the original error", err)
	}
	if strings.Contains(err.Error(), "credential ") {
		t.Errorf("non-auth error must not be attributed to a credential: %q", err)
	}
}

// TestAttributedTokenSource_PassesThroughSuccess proves the happy path is
// untouched.
func TestAttributedTokenSource_PassesThroughSuccess(t *testing.T) {
	want := &oauth2.Token{AccessToken: "at"}
	ats := &attributedTokenSource{base: oauth2.StaticTokenSource(want), ref: "google-readonly/default", source: config.RefSourceDefault}
	got, err := ats.Token()
	if err != nil || got.AccessToken != want.AccessToken {
		t.Fatalf("Token() = (%v, %v), want (%v, nil)", got, err, want)
	}
}

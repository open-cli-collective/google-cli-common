package profilescmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"golang.org/x/oauth2"

	"github.com/open-cli-collective/google-cli-common/config"
	"github.com/open-cli-collective/google-cli-common/credtest"
	"github.com/open-cli-collective/google-cli-common/identitycache"
	"github.com/open-cli-collective/google-cli-common/keychain"
)

func TestMain(m *testing.M) {
	config.RegisterForTest()
	os.Exit(m.Run())
}

// capture redirects os.Stdout for f (the command prints with fmt.Printf).
func capture(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() { var b bytes.Buffer; _, _ = io.Copy(&b, r); done <- b.String() }()
	func() {
		defer func() {
			os.Stdout = orig
			_ = w.Close()
		}()
		f()
	}()
	return <-done
}

// seedToken stores a token under the given profile of the test service.
func seedToken(t *testing.T, profile string) {
	t.Helper()
	st, err := keychain.OpenRef("google-readonly/" + profile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if err := st.SetToken(&oauth2.Token{AccessToken: "A-" + profile, RefreshToken: "R"}); err != nil {
		t.Fatal(err)
	}
}

func withVerify(t *testing.T, f func(ctx context.Context, ref string) (string, error)) {
	t.Helper()
	orig := VerifyRef
	VerifyRef = f
	t.Cleanup(func() { VerifyRef = orig })
}

func TestNewCommandSurface(t *testing.T) {
	cmd := NewCommand()
	if cmd.Use != "profiles" {
		t.Errorf("Use = %q, want profiles", cmd.Use)
	}
	var names []string
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	for _, want := range []string{"list", "use"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Errorf("missing subcommand %q (have %v)", want, names)
		}
	}
}

func TestListFlags(t *testing.T) {
	cmd := newListCommand()
	if f := cmd.Flags().Lookup("json"); f == nil || f.Shorthand != "j" {
		t.Errorf("expected --json/-j flag, got %+v", f)
	}
	if f := cmd.Flags().Lookup("check"); f == nil {
		t.Error("expected --check flag")
	}
}

// TestRunList_ListsStoredProfiles is the headline behavior: every stored
// profile visible in one command, active one marked, no keychain dumping.
func TestRunList_ListsStoredProfiles(t *testing.T) {
	credtest.Setup(t)
	seedToken(t, "default")
	seedToken(t, "work")

	out := capture(t, func() {
		if err := runList(context.Background(), false, false); err != nil {
			t.Errorf("runList: %v", err)
		}
	})

	for _, want := range []string{"default", "work", "present", "Active: google-readonly/default", "profiles use"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// The active row carries the '*' marker.
	activeMarked := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "*") && strings.Contains(line, "default") {
			activeMarked = true
		}
	}
	if !activeMarked {
		t.Errorf("active profile not marked with '*':\n%s", out)
	}
}

// TestRunList_FreshInstall shows the active (default) profile even before any
// token exists, with the init hint — a fresh user must not see an empty
// table with no way forward.
func TestRunList_FreshInstall(t *testing.T) {
	credtest.Setup(t)

	out := capture(t, func() {
		if err := runList(context.Background(), false, false); err != nil {
			t.Errorf("runList: %v", err)
		}
	})

	for _, want := range []string{"default", "missing", "init"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

// TestRunList_ShowsCachedEmail proves the resolved-account column comes from
// the identity cache without any API call.
func TestRunList_ShowsCachedEmail(t *testing.T) {
	credtest.Setup(t)
	seedToken(t, "default")
	if err := identitycache.Put("default", "user@example.com"); err != nil {
		t.Fatal(err)
	}

	out := capture(t, func() {
		if err := runList(context.Background(), false, false); err != nil {
			t.Errorf("runList: %v", err)
		}
	})
	if !strings.Contains(out, "user@example.com") {
		t.Errorf("output missing cached email:\n%s", out)
	}
}

func TestRunList_JSON(t *testing.T) {
	credtest.Setup(t)
	seedToken(t, "default")

	out := capture(t, func() {
		if err := runList(context.Background(), true, false); err != nil {
			t.Errorf("runList: %v", err)
		}
	})
	for _, want := range []string{`"profile": "default"`, `"ref": "google-readonly/default"`, `"active": true`, `"token_present": true`} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON missing %q:\n%s", want, out)
		}
	}
}

// TestRunList_Check is the incident scenario: one stale profile, one healthy.
// --check must say so per profile and cache the healthy one's email.
func TestRunList_Check(t *testing.T) {
	credtest.Setup(t)
	seedToken(t, "default")
	seedToken(t, "work")
	seedToken(t, "broken")

	withVerify(t, func(_ context.Context, ref string) (string, error) {
		switch ref {
		case "google-readonly/default":
			return "", &oauth2.RetrieveError{
				Response:         &http.Response{StatusCode: http.StatusBadRequest},
				ErrorCode:        "invalid_grant",
				ErrorDescription: "Token has been expired or revoked.",
			}
		case "google-readonly/work":
			return "work@example.com", nil
		default:
			return "", errors.New("dial tcp: connection refused\nsecond line noise")
		}
	})

	out := capture(t, func() {
		if err := runList(context.Background(), false, true); err != nil {
			t.Errorf("runList: %v", err)
		}
	})

	for _, want := range []string{"expired or revoked", "ok", "error: dial tcp: connection refused", "work@example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "second line noise") {
		t.Errorf("multi-line error leaked past firstLine:\n%s", out)
	}
	// Healthy check must refresh the identity cache.
	if got := identitycache.Load()["work"].Email; got != "work@example.com" {
		t.Errorf("identity cache after check = %q, want work@example.com", got)
	}
}

func TestRunUse_SwitchesActiveProfile(t *testing.T) {
	credtest.Setup(t)
	seedToken(t, "default")
	seedToken(t, "work")

	out := capture(t, func() {
		if err := runUse("work"); err != nil {
			t.Errorf("runUse: %v", err)
		}
	})
	if !strings.Contains(out, "Active profile is now google-readonly/work.") {
		t.Errorf("missing confirmation:\n%s", out)
	}

	cfg, err := config.LoadConfigForRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CredentialRef != "google-readonly/work" {
		t.Errorf("credential_ref = %q, want google-readonly/work", cfg.CredentialRef)
	}
	// Switching must not touch the previous profile's token.
	st, err := keychain.OpenRef("google-readonly/default")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if has, _ := st.HasToken(); !has {
		t.Error("switching away removed the previous profile's token")
	}
}

// TestRunUse_MissingTokenWarnsButProceeds pins the add-second-account flow:
// `use work` then `init`. The warning names existing profiles so a typo is
// visible immediately.
func TestRunUse_MissingTokenWarnsButProceeds(t *testing.T) {
	credtest.Setup(t)
	seedToken(t, "default")

	out := capture(t, func() {
		if err := runUse("wrk"); err != nil {
			t.Errorf("runUse: %v", err)
		}
	})
	for _, want := range []string{"no token is stored for google-readonly/wrk", "Existing profiles: default", "init"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	cfg, _ := config.LoadConfigForRuntime()
	if cfg.CredentialRef != "google-readonly/wrk" {
		t.Errorf("credential_ref = %q, want google-readonly/wrk", cfg.CredentialRef)
	}
}

func TestRunUse_AlreadyActive(t *testing.T) {
	credtest.Setup(t)
	seedToken(t, "default")

	out := capture(t, func() {
		if err := runUse("default"); err != nil {
			t.Errorf("runUse: %v", err)
		}
	})
	if !strings.Contains(out, "already the active profile") {
		t.Errorf("expected already-active message:\n%s", out)
	}
}

func TestRunUse_FullRefSameService(t *testing.T) {
	credtest.Setup(t)
	seedToken(t, "work")

	if err := runUseQuiet(t, "google-readonly/work"); err != nil {
		t.Fatalf("runUse(full ref): %v", err)
	}
	cfg, _ := config.LoadConfigForRuntime()
	if cfg.CredentialRef != "google-readonly/work" {
		t.Errorf("credential_ref = %q, want google-readonly/work", cfg.CredentialRef)
	}
}

func TestRunUse_CrossServiceRefRejected(t *testing.T) {
	credtest.Setup(t)

	err := runUseQuiet(t, "google-readwrite/work")
	if err == nil || !strings.Contains(err.Error(), "google-readwrite") {
		t.Fatalf("expected cross-service rejection, got %v", err)
	}
}

func TestRunUse_InvalidProfileRejected(t *testing.T) {
	credtest.Setup(t)

	err := runUseQuiet(t, "bad.profile")
	if err == nil {
		t.Fatal("expected invalid-profile error")
	}
}

// runUseQuiet runs runUse with stdout swallowed (these cases assert on error
// or config state, not output).
func runUseQuiet(t *testing.T, arg string) error {
	t.Helper()
	var err error
	capture(t, func() { err = runUse(arg) })
	return err
}

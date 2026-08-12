package keychain

import (
	"testing"

	"golang.org/x/oauth2"

	"github.com/open-cli-collective/google-cli-common/config"
	"github.com/open-cli-collective/google-cli-common/credtest"
)

// TestListProfilesAndHasTokenFor covers the cross-profile read surface that
// backs `profiles list`: enumeration reflects stored reality, and presence
// checks work against profiles other than the one the Store was opened for.
func TestListProfilesAndHasTokenFor(t *testing.T) {
	credtest.Setup(t)

	// Fresh service: nothing stored yet.
	st, err := openWith(testCfg(), false, false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = st.Close() }()
	if ps, lerr := st.ListProfiles(); lerr != nil || len(ps) != 0 {
		t.Fatalf("fresh ListProfiles = (%v, %v), want empty", ps, lerr)
	}

	// Seed tokens under the active and a second profile.
	if err := st.SetToken(&oauth2.Token{AccessToken: "A", RefreshToken: "R"}); err != nil {
		t.Fatalf("seed default: %v", err)
	}
	work, err := openWith(&config.Config{CredentialRef: "google-readonly/work"}, false, false)
	if err != nil {
		t.Fatalf("open work: %v", err)
	}
	if err := work.SetToken(&oauth2.Token{AccessToken: "B", RefreshToken: "S"}); err != nil {
		t.Fatalf("seed work: %v", err)
	}
	_ = work.Close()

	ps, err := st.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(ps) != 2 || ps[0] != "default" || ps[1] != "work" {
		t.Fatalf("ListProfiles = %v, want [default work] (sorted)", ps)
	}

	// Cross-profile presence from the default-bound store handle.
	if has, herr := st.HasTokenFor("work"); herr != nil || !has {
		t.Fatalf("HasTokenFor(work) = (%v, %v), want (true, nil)", has, herr)
	}
	if has, herr := st.HasTokenFor("absent"); herr != nil || has {
		t.Fatalf("HasTokenFor(absent) = (%v, %v), want (false, nil)", has, herr)
	}
}

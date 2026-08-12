package keychain

import (
	"testing"

	"github.com/open-cli-collective/google-cli-common/config"
)

// resetCredRefOverride keeps the package-level --ref override clean across
// tests so a leaked value can't tilt the next.
func resetCredRefOverride(t *testing.T) {
	t.Helper()
	SetCredentialRefOverride("", false)
	t.Cleanup(func() { SetCredentialRefOverride("", false) })
}

// TestCredentialRefEnvVar pins the derived env-var name to the documented
// <SERVICE>_CREDENTIAL_REF, tracking the same prefix as the backend env var.
func TestCredentialRefEnvVar(t *testing.T) {
	if got := CredentialRefEnvVar(); got != "GOOGLE_READONLY_CREDENTIAL_REF" {
		t.Errorf("CredentialRefEnvVar() = %q, want %q", got, "GOOGLE_READONLY_CREDENTIAL_REF")
	}
}

// TestEffectiveRef_Precedence proves --ref flag > env > config, and that an
// override is reported (so the caller suppresses the one-time migration).
func TestEffectiveRef_Precedence(t *testing.T) {
	const cfgRef = "google-readonly/cfg"

	t.Run("config only", func(t *testing.T) {
		resetCredRefOverride(t)
		t.Setenv(CredentialRefEnvVar(), "")
		ref, src, ov := effectiveRef(cfgRef)
		if ref != cfgRef || ov || src != "" {
			t.Errorf("got (%q,%q,%v), want (%q,\"\",false)", ref, src, ov, cfgRef)
		}
	})

	t.Run("env overrides config", func(t *testing.T) {
		resetCredRefOverride(t)
		t.Setenv(CredentialRefEnvVar(), "google-readonly/env")
		ref, src, ov := effectiveRef(cfgRef)
		if ref != "google-readonly/env" || !ov || src != config.RefSourceEnv {
			t.Errorf("got (%q,%q,%v), want (google-readonly/env,env,true)", ref, src, ov)
		}
	})

	t.Run("flag overrides env and config", func(t *testing.T) {
		resetCredRefOverride(t)
		t.Setenv(CredentialRefEnvVar(), "google-readonly/env")
		SetCredentialRefOverride("google-readonly/flag", true)
		ref, src, ov := effectiveRef(cfgRef)
		if ref != "google-readonly/flag" || !ov || src != config.RefSourceFlag {
			t.Errorf("got (%q,%q,%v), want (google-readonly/flag,flag,true)", ref, src, ov)
		}
	})

	t.Run("flag set but empty does not override", func(t *testing.T) {
		resetCredRefOverride(t)
		t.Setenv(CredentialRefEnvVar(), "")
		SetCredentialRefOverride("", true) // Changed=true but no value
		ref, _, ov := effectiveRef(cfgRef)
		if ref != cfgRef || ov {
			t.Errorf("got (%q,%v), want (%q,false) — empty --ref must fall through", ref, ov, cfgRef)
		}
	})
}

// TestApplyCredentialRefOverride proves the safety-critical part open() relies
// on: a present override swaps cfg.CredentialRef AND forces runMigration=false
// (so the one-time legacy migration never runs against an arbitrary
// --ref/env-selected profile), while no override leaves both untouched. This is
// the open()-side coverage the pure effectiveRef/wiring tests don't provide.
func TestApplyCredentialRefOverride(t *testing.T) {
	const cfgRef = "google-readonly/cfg"

	t.Run("no override: cfg untouched, runMigration passthrough", func(t *testing.T) {
		resetCredRefOverride(t)
		t.Setenv(CredentialRefEnvVar(), "")
		cfg := &config.Config{CredentialRef: cfgRef}
		if rm := applyCredentialRefOverride(cfg, true); !rm {
			t.Errorf("runMigration = %v, want true (passthrough)", rm)
		}
		if cfg.CredentialRef != cfgRef {
			t.Errorf("cfg.CredentialRef = %q, want unchanged %q", cfg.CredentialRef, cfgRef)
		}
		// passthrough must preserve a false caller value too (OpenNoMigrate path)
		if rm := applyCredentialRefOverride(cfg, false); rm {
			t.Errorf("runMigration = %v, want false (passthrough of caller's false)", rm)
		}
	})

	t.Run("flag override: cfg swapped, migration suppressed", func(t *testing.T) {
		resetCredRefOverride(t)
		t.Setenv(CredentialRefEnvVar(), "")
		SetCredentialRefOverride("google-readonly/flag", true)
		cfg := &config.Config{CredentialRef: cfgRef}
		if rm := applyCredentialRefOverride(cfg, true); rm {
			t.Errorf("runMigration = %v, want false (override must suppress migration)", rm)
		}
		if cfg.CredentialRef != "google-readonly/flag" {
			t.Errorf("cfg.CredentialRef = %q, want google-readonly/flag", cfg.CredentialRef)
		}
		if src := cfg.CredentialRefSource(); src != config.RefSourceFlag {
			t.Errorf("CredentialRefSource = %q, want flag", src)
		}
	})

	t.Run("env override: cfg swapped, migration suppressed", func(t *testing.T) {
		resetCredRefOverride(t)
		t.Setenv(CredentialRefEnvVar(), "google-readonly/env")
		cfg := &config.Config{CredentialRef: cfgRef}
		if rm := applyCredentialRefOverride(cfg, true); rm {
			t.Errorf("runMigration = %v, want false (env override must suppress migration)", rm)
		}
		if cfg.CredentialRef != "google-readonly/env" {
			t.Errorf("cfg.CredentialRef = %q, want google-readonly/env", cfg.CredentialRef)
		}
		if src := cfg.CredentialRefSource(); src != config.RefSourceEnv {
			t.Errorf("CredentialRefSource = %q, want env", src)
		}
	})
}

// TestDescribeRefSource pins the human labels used in attributed auth errors
// and `config show` — including the dynamically derived env-var name.
func TestDescribeRefSource(t *testing.T) {
	cases := []struct {
		src  config.RefSource
		want string
	}{
		{config.RefSourceFlag, "--ref flag"},
		{config.RefSourceEnv, "GOOGLE_READONLY_CREDENTIAL_REF environment variable"},
		{config.RefSourceConfig, "config.yml credential_ref"},
		{config.RefSourceDefault, "built-in default; config.yml sets no credential_ref"},
		{config.RefSourceExplicit, "explicitly selected ref"},
		{config.RefSource(""), "unknown source"},
	}
	for _, tc := range cases {
		if got := DescribeRefSource(tc.src); got != tc.want {
			t.Errorf("DescribeRefSource(%q) = %q, want %q", tc.src, got, tc.want)
		}
	}
}

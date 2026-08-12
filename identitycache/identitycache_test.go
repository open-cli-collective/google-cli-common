package identitycache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/open-cli-collective/google-cli-common/config"
	"github.com/open-cli-collective/google-cli-common/credtest"
)

func TestMain(m *testing.M) {
	config.RegisterForTest()
	os.Exit(m.Run())
}

func TestLoadAbsentIsEmpty(t *testing.T) {
	credtest.Setup(t)
	if got := Load(); len(got) != 0 {
		t.Errorf("Load() on fresh cache = %v, want empty", got)
	}
}

func TestPutLoadRoundtrip(t *testing.T) {
	credtest.Setup(t)
	before := time.Now().UTC().Add(-time.Second)
	if err := Put("default", "user@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := Put("work", "work@example.com"); err != nil {
		t.Fatal(err)
	}
	// Second Put must preserve the first profile's entry (load-modify-save).
	m := Load()
	if m["default"].Email != "user@example.com" || m["work"].Email != "work@example.com" {
		t.Errorf("Load() = %v, want both entries", m)
	}
	if m["default"].VerifiedAt.Before(before) {
		t.Errorf("VerifiedAt = %v, want recent", m["default"].VerifiedAt)
	}
}

func TestPutOverwritesProfile(t *testing.T) {
	credtest.Setup(t)
	if err := Put("default", "old@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := Put("default", "new@example.com"); err != nil {
		t.Fatal(err)
	}
	if got := Load()["default"].Email; got != "new@example.com" {
		t.Errorf("Email = %q, want new@example.com", got)
	}
}

func TestPutRejectsEmpty(t *testing.T) {
	credtest.Setup(t)
	if err := Put("", "a@b.com"); err == nil {
		t.Error("Put with empty profile should fail")
	}
	if err := Put("default", ""); err == nil {
		t.Error("Put with empty email should fail")
	}
}

func TestLoadToleratesCorruptFile(t *testing.T) {
	credtest.Setup(t)
	dir, err := config.GetCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	// The envelope file lives at <cachedir>/<instanceKey>/<resource>.json.
	sub := filepath.Join(dir, "default")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "profile_identities.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Load(); len(got) != 0 {
		t.Errorf("Load() on corrupt cache = %v, want empty (best-effort)", got)
	}
	// And Put must recover by rewriting the file.
	if err := Put("default", "user@example.com"); err != nil {
		t.Fatal(err)
	}
	if got := Load()["default"].Email; got != "user@example.com" {
		t.Errorf("recovery Put/Load = %q, want user@example.com", got)
	}
}

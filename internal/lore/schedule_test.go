package lore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScheduledVerificationRunsFullOncePerLocalMonth(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".lore", "verification.json")
	august := time.Date(2026, 8, 17, 3, 0, 0, 0, time.Local)

	assertScheduledVerificationPlan(t, path, august, true, "2026-08")

	if err := recordScheduledFullVerification(path, "2026-08"); err != nil {
		t.Fatalf("record August verification: %v", err)
	}

	assertScheduledVerificationPlan(t, path, august.AddDate(0, 0, 1), false, "2026-08")

	september := time.Date(2026, 9, 19, 3, 0, 0, 0, time.Local)
	assertScheduledVerificationPlan(t, path, september, true, "2026-09")

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat verification state: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Fatalf("verification state mode = %o, want 600", info.Mode().Perm())
	}
}

func assertScheduledVerificationPlan(t *testing.T, path string, now time.Time, wantFull bool, wantMonth string) {
	t.Helper()

	full, month, err := scheduledVerificationPlan(path, now)
	if err != nil || full != wantFull || month != wantMonth {
		t.Fatalf("scheduled plan = %v, %q, %v; want %v, %q", full, month, err, wantFull, wantMonth)
	}
}

func TestScheduledVerificationRebuildsInvalidDerivedState(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "verification.json")
	if err := os.WriteFile(path, []byte("not-json\n"), 0o600); err != nil {
		t.Fatalf("write invalid verification state: %v", err)
	}

	full, month, err := scheduledVerificationPlan(path, time.Date(2026, 10, 12, 3, 0, 0, 0, time.Local))
	if err != nil || !full || month != "2026-10" {
		t.Fatalf("rebuild plan = %v, %q, %v", full, month, err)
	}
}

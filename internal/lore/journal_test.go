package lore

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSegmentPath(t *testing.T) {
	t.Parallel()

	timestamp := time.Date(2026, 8, 31, 23, 30, 0, 0, time.FixedZone("offset", -2*60*60))
	got := SegmentPath("/journal", timestamp)

	want := filepath.FromSlash("/journal/2026/09.jsonl")
	if got != want {
		t.Fatalf("SegmentPath() = %q, want %q", got, want)
	}
}

func TestJournalAppendLoadAndDeduplicate(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	event := validMilestoneEvent()
	result := appendJournalEvent(t, journal, event)

	if !result.Recorded || result.Event.ID != event.ID {
		t.Fatalf("Append() result = %#v", result)
	}

	duplicate := event
	duplicate.ID = nextEventID(t, duplicate.RecordedAt, 3)
	duplicate.Summary = "This supplied summary must not replace the published summary"

	result = appendJournalEvent(t, journal, duplicate)

	if result.Recorded || result.Event.ID != event.ID || result.Event.Summary != event.Summary {
		t.Fatalf("duplicate Append() result = %#v", result)
	}

	if journal.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", journal.Count())
	}

	segmentPath := SegmentPath(config.JournalRoot, event.RecordedAt)
	if got := countLines(t, segmentPath); got != 1 {
		t.Fatalf("segment lines = %d, want 1", got)
	}

	assertSegmentPermissions(t, segmentPath)
	assertReloadedMilestone(t, config, event)
}

func TestJournalAppendRequiresLoad(t *testing.T) {
	t.Parallel()

	journal := NewJournal(testJournalConfig(t))
	if _, err := journal.Append(context.Background(), validMilestoneEvent()); !errors.Is(err, ErrJournalNotLoaded) {
		t.Fatalf("Append() error = %v, want ErrJournalNotLoaded", err)
	}
}

func appendJournalEvent(t *testing.T, journal *Journal, event Event) AppendResult {
	t.Helper()

	result, err := journal.Append(context.Background(), event)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	return result
}

func assertSegmentPermissions(t *testing.T, segmentPath string) {
	t.Helper()

	info, err := os.Stat(segmentPath)
	if err != nil {
		t.Fatalf("stat segment: %v", err)
	}

	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("segment mode = %o, want 600", got)
	}
}

func assertReloadedMilestone(t *testing.T, config Config, event Event) {
	t.Helper()

	reloaded := NewJournal(config)
	if err := reloaded.Load(context.Background()); err != nil {
		t.Fatalf("reload journal: %v", err)
	}

	if reloaded.Count() != 1 {
		t.Fatalf("reloaded Count() = %d, want 1", reloaded.Count())
	}

	got, ok := reloaded.Event(event.ID)
	if !ok || got.ID != event.ID {
		t.Fatalf("Event() = %#v, %v", got, ok)
	}
}

func TestJournalMonthlySegments(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	august := validMilestoneEvent()
	september := validMilestoneEvent()
	september.RecordedAt = time.Date(2026, 9, 1, 0, 0, 1, 0, time.UTC)
	september.ID = nextEventID(t, september.RecordedAt, 3)
	september.Properties.ObjectID = "1123456789abcdef0123456789abcdef01234567"

	for _, event := range []Event{august, september} {
		if _, err := journal.Append(context.Background(), event); err != nil {
			t.Fatalf("Append(%s) error = %v", event.ID, err)
		}

		if _, err := os.Stat(SegmentPath(config.JournalRoot, event.RecordedAt)); err != nil {
			t.Fatalf("stat segment for %s: %v", event.ID, err)
		}
	}

	if journal.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", journal.Count())
	}
}

func TestJournalCorrectionChain(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	milestone := validMilestoneEvent()
	if _, err := journal.Append(context.Background(), milestone); err != nil {
		t.Fatalf("append milestone: %v", err)
	}

	correction := milestone
	correction.RecordedAt = correction.RecordedAt.Add(time.Hour)
	correction.ID = nextEventID(t, correction.RecordedAt, 4)
	correction.Summary = "Clarified reusable behavior"
	correction.Supersedes = milestone.ID

	correction.CorrectionReason = "Clarify the supported behavior"
	if _, err := journal.Append(context.Background(), correction); err != nil {
		t.Fatalf("append correction: %v", err)
	}

	effective, ok := journal.Effective(milestone.ID)
	if !ok || effective.ID != correction.ID {
		t.Fatalf("Effective() = %#v, %v", effective, ok)
	}

	stale := correction
	stale.RecordedAt = stale.RecordedAt.Add(time.Hour)
	stale.ID = nextEventID(t, stale.RecordedAt, 5)

	stale.Supersedes = milestone.ID
	if _, err := journal.Append(context.Background(), stale); err == nil {
		t.Fatal("expected stale correction target error")
	}

	reloaded := NewJournal(config)
	if err := reloaded.Load(context.Background()); err != nil {
		t.Fatalf("reload journal: %v", err)
	}

	effective, ok = reloaded.Effective(milestone.ID)
	if !ok || effective.ID != correction.ID {
		t.Fatalf("reloaded Effective() = %#v, %v", effective, ok)
	}
}

func TestJournalLoadRejectsWrongSegment(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)
	if err := os.MkdirAll(filepath.Join(config.JournalRoot, "2026"), 0o700); err != nil {
		t.Fatalf("create segment directory: %v", err)
	}

	event := validMilestoneEvent()

	line, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	wrongPath := filepath.Join(config.JournalRoot, "2026", "09.jsonl")
	if err := os.WriteFile(wrongPath, append(line, '\n'), 0o600); err != nil {
		t.Fatalf("write segment: %v", err)
	}

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("Load() error = %v, want ErrJournalCorrupt", err)
	}
}

func testJournalConfig(t *testing.T) Config {
	t.Helper()

	root := t.TempDir()
	filesRoot := filepath.Join(root, ".lore")

	return Config{
		ConfigRoot:            filepath.Join(root, ".config", "lore"),
		FilesRoot:             filesRoot,
		JournalRoot:           filepath.Join(filesRoot, "events"),
		PendingPath:           filepath.Join(filesRoot, "pending.json"),
		VerificationStatePath: filepath.Join(filesRoot, "verification.json"),
	}
}

func countLines(t *testing.T, path string) int {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}

	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("close %s: %v", path, err)
		}
	})

	count := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}

	return count
}

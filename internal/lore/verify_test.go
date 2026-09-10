package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestVerifierChecksActiveAndFullScopesAcrossSegments(t *testing.T) {
	t.Parallel()

	journal, verifier, milestone, correction := newCrossSegmentVerifier(t)

	active, err := verifier.Verify(context.Background(), VerifyParams{})
	if err != nil {
		t.Fatalf("active Verify() error = %v", err)
	}

	if active.Scope != VerifyScopeActive || active.SegmentCount != 1 || active.EventCount != 1 {
		t.Fatalf("active Verify() = %#v", active)
	}

	full, err := verifier.Verify(context.Background(), VerifyParams{Full: true})
	if err != nil {
		t.Fatalf("full Verify() error = %v", err)
	}

	if full.Scope != VerifyScopeFull || full.SegmentCount != 2 || full.EventCount != 2 {
		t.Fatalf("full Verify() = %#v", full)
	}

	assertVerifierDidNotMutateJournal(t, journal, milestone, correction)
}

func TestActiveVerifierDoesNotReadHistoricalSegments(t *testing.T) {
	t.Parallel()

	journal, verifier, milestone, _ := newCrossSegmentVerifier(t)
	historicalPath := SegmentPath(journal.config.JournalRoot, milestone.RecordedAt)
	corruptFirstByte(t, historicalPath)

	if _, err := verifier.Verify(context.Background(), VerifyParams{}); err != nil {
		t.Fatalf("active Verify() error = %v", err)
	}

	if _, err := verifier.Verify(context.Background(), VerifyParams{Full: true}); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("full Verify() error = %v, want ErrJournalCorrupt", err)
	}
}

func TestVerifierReportsExactLineAndByteOffset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mutate     func(*testing.T, string)
		wantLine   int
		wantOffset int64
	}{
		{name: "invalid UTF-8", mutate: corruptFirstByte, wantLine: 1, wantOffset: 0},
		{name: "blank line", mutate: prependBlankLine, wantLine: 1, wantOffset: 0},
		{name: "missing LF", mutate: removeTerminalLF, wantLine: 1, wantOffset: -1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			journal, verifier, event := newSingleEventVerifier(t)
			segmentPath := SegmentPath(journal.config.JournalRoot, event.RecordedAt)
			test.mutate(t, segmentPath)

			_, err := verifier.Verify(context.Background(), VerifyParams{Full: true})
			assertIntegrityLocation(t, err, segmentPath, test.wantLine, test.wantOffset)
		})
	}
}

func TestVerifierRejectsGlobalInvariantViolations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		append func(*testing.T, string, Event)
		reason string
	}{
		{
			name: "duplicate event ID",
			append: func(t *testing.T, path string, event Event) {
				appendRawEvent(t, path, event)
			},
			reason: "duplicate event ID",
		},
		{
			name: "duplicate natural key",
			append: func(t *testing.T, path string, event Event) {
				event.ID = nextEventID(t, event.RecordedAt.Add(time.Millisecond), 21)
				event.RecordedAt = event.RecordedAt.Add(time.Millisecond)
				appendRawEvent(t, path, event)
			},
			reason: "duplicate natural key",
		},
		{
			name: "missing correction target",
			append: func(t *testing.T, path string, event Event) {
				event.ID = nextEventID(t, event.RecordedAt.Add(time.Millisecond), 22)
				event.RecordedAt = event.RecordedAt.Add(time.Millisecond)
				event.Supersedes = "01K1Z8V7BMMN6W7Y6R8QJHA9ZZ"
				event.CorrectionReason = "Target must precede correction"
				appendRawEvent(t, path, event)
			},
			reason: "does not exist",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			journal, verifier, event := newSingleEventVerifier(t)
			segmentPath := SegmentPath(journal.config.JournalRoot, event.RecordedAt)
			test.append(t, segmentPath, event)

			_, err := verifier.Verify(context.Background(), VerifyParams{Full: true})
			assertIntegrityReason(t, err, segmentPath, 2, test.reason)
		})
	}
}

func TestVerifierRejectsWrongSegmentAndCorrectionIdentity(t *testing.T) {
	t.Parallel()

	t.Run("wrong segment", func(t *testing.T) {
		t.Parallel()

		journal, verifier, event := newSingleEventVerifier(t)
		segmentPath := SegmentPath(journal.config.JournalRoot, event.RecordedAt)
		event.RecordedAt = event.RecordedAt.AddDate(0, 1, 0)
		event.ID = nextEventID(t, event.RecordedAt, 24)
		rewriteRawEvents(t, segmentPath, event)

		_, err := verifier.Verify(context.Background(), VerifyParams{Full: true})
		assertIntegrityReason(t, err, segmentPath, 1, "event belongs")
	})

	t.Run("correction snapshot", func(t *testing.T) {
		t.Parallel()

		journal, verifier, event := newSingleEventVerifier(t)
		segmentPath := SegmentPath(journal.config.JournalRoot, event.RecordedAt)
		correction := event
		correction.ID = nextEventID(t, event.RecordedAt.Add(time.Millisecond), 23)
		correction.RecordedAt = event.RecordedAt.Add(time.Millisecond)
		correction.Supersedes = event.ID
		correction.CorrectionReason = "Detect changed natural identity"
		correction.Properties.ObjectID = "1123456789abcdef0123456789abcdef01234567"
		appendRawEvent(t, segmentPath, correction)

		_, err := verifier.Verify(context.Background(), VerifyParams{Full: true})
		assertIntegrityReason(t, err, segmentPath, 2, "identity does not match")
	})
}

func TestFullVerifierPreservesLoadFailureOutsideCanonicalSegments(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)
	if err := prepareJournalDirectories(config); err != nil {
		t.Fatalf("prepare journal: %v", err)
	}

	if err := os.WriteFile(config.PendingPath, []byte("not-json\n"), 0o600); err != nil {
		t.Fatalf("write corrupt pending state: %v", err)
	}

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("Load() error = %v", err)
	}

	_, err := NewVerifier(journal).Verify(context.Background(), VerifyParams{Full: true})
	if !errors.Is(err, ErrJournalCorrupt) || !strings.Contains(err.Error(), config.PendingPath) {
		t.Fatalf("Verify() error = %v", err)
	}
}

func newCrossSegmentVerifier(t *testing.T) (*Journal, *Verifier, Event, Event) {
	t.Helper()

	journal := NewJournal(testJournalConfig(t))
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("load journal: %v", err)
	}

	milestone := validMilestoneEvent()
	milestone.RecordedAt = time.Date(2026, 8, 31, 23, 59, 0, 0, time.UTC)
	milestone.ID = nextEventID(t, milestone.RecordedAt, 1)
	appendJournalEvent(t, journal, milestone)

	correction := milestone
	correction.RecordedAt = time.Date(2026, 9, 1, 0, 1, 0, 0, time.UTC)
	correction.ID = nextEventID(t, correction.RecordedAt, 2)
	correction.Supersedes = milestone.ID
	correction.CorrectionReason = "Clarify behavior across segments"
	appendJournalEvent(t, journal, correction)

	verifier := NewVerifier(journal)
	verifier.now = func() time.Time {
		return time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	}

	return journal, verifier, milestone, correction
}

func newSingleEventVerifier(t *testing.T) (*Journal, *Verifier, Event) {
	t.Helper()

	journal := NewJournal(testJournalConfig(t))
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("load journal: %v", err)
	}

	event := validMilestoneEvent()
	appendJournalEvent(t, journal, event)

	return journal, NewVerifier(journal), event
}

func nextEventID(t *testing.T, recordedAt time.Time, marker byte) EventID {
	t.Helper()

	id, err := NewEventID(recordedAt, bytes.NewReader(bytes.Repeat([]byte{marker}, 10)))
	if err != nil {
		t.Fatalf("generate event ID: %v", err)
	}

	return id
}

func appendRawEvent(t *testing.T, path string, event Event) {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal raw event: %v", err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open raw segment: %v", err)
	}

	if _, err := file.Write(append(encoded, '\n')); err != nil {
		t.Fatalf("append raw event: %v", closeJournalFile(file, err))
	}

	if err := closeJournalFile(file, nil); err != nil {
		t.Fatalf("close raw segment: %v", err)
	}
}

func rewriteRawEvents(t *testing.T, path string, events ...Event) {
	t.Helper()

	var content bytes.Buffer

	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal raw event: %v", err)
		}

		content.Write(encoded)
		content.WriteByte('\n')
	}

	if err := os.WriteFile(path, content.Bytes(), 0o600); err != nil {
		t.Fatalf("rewrite raw segment: %v", err)
	}
}

func corruptFirstByte(t *testing.T, path string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}

	content[0] = 0xff
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("corrupt segment: %v", err)
	}
}

func prependBlankLine(t *testing.T, path string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}

	if err := os.WriteFile(path, append([]byte{'\n'}, content...), 0o600); err != nil {
		t.Fatalf("prepend blank line: %v", err)
	}
}

func removeTerminalLF(t *testing.T, path string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}

	if err := os.WriteFile(path, content[:len(content)-1], 0o600); err != nil {
		t.Fatalf("remove terminal LF: %v", err)
	}
}

func assertIntegrityLocation(t *testing.T, err error, segment string, line int, offset int64) {
	t.Helper()

	var integrityError *IntegrityError
	if !errors.As(err, &integrityError) {
		t.Fatalf("Verify() error = %v, want IntegrityError", err)
	}

	if offset < 0 {
		info, statErr := os.Stat(segment)
		if statErr != nil {
			t.Fatalf("stat corrupted segment: %v", statErr)
		}

		offset = info.Size() - 1
	}

	if integrityError.Segment != segment || integrityError.Line != line || integrityError.ByteOffset != offset {
		t.Fatalf("IntegrityError = %#v", integrityError)
	}
}

func assertIntegrityReason(t *testing.T, err error, segment string, line int, reason string) {
	t.Helper()

	var integrityError *IntegrityError
	if !errors.As(err, &integrityError) {
		t.Fatalf("Verify() error = %v, want IntegrityError", err)
	}

	if integrityError.Segment != segment || integrityError.Line != line || !strings.Contains(integrityError.Reason, reason) {
		t.Fatalf("IntegrityError = %#v", integrityError)
	}
}

func assertVerifierDidNotMutateJournal(t *testing.T, journal *Journal, events ...Event) {
	t.Helper()

	if journal.Count() != len(events) {
		t.Fatalf("journal count = %d, want %d", journal.Count(), len(events))
	}

	for _, event := range events {
		if stored, ok := journal.Event(event.ID); !ok || !reflect.DeepEqual(stored, event) {
			t.Fatalf("stored event = %#v, %v", stored, ok)
		}
	}
}

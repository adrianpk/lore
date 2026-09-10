package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type pendingRecoveryCase struct {
	name      string
	writeTail func(*testing.T, string, []byte)
	wantErr   bool
}

func TestJournalRecoversPendingStates(t *testing.T) {
	t.Parallel()

	tests := []pendingRecoveryCase{
		{name: "missing append"},
		{name: "complete append", writeTail: writeCompletePendingTail},
		{name: "partial append", writeTail: writePartialPendingTail},
		{name: "mismatched append", writeTail: writeMismatchedPendingTail, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assertPendingRecovery(t, test)
		})
	}
}

func assertPendingRecovery(t *testing.T, test pendingRecoveryCase) {
	t.Helper()

	config := testJournalConfig(t)
	if err := prepareJournalDirectories(config); err != nil {
		t.Fatalf("prepare journal: %v", err)
	}

	event := validMilestoneEvent()
	segmentPath, line, operation := prepareTestPending(t, config, event)

	if err := writePendingOperation(config.PendingPath, operation); err != nil {
		t.Fatalf("write pending operation: %v", err)
	}

	if test.writeTail != nil {
		test.writeTail(t, segmentPath, line)
	}

	journal := NewJournal(config)
	err := journal.Load(context.Background())

	if test.wantErr {
		assertBlockedPendingRecovery(t, config.PendingPath, err)

		return
	}

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if journal.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", journal.Count())
	}

	assertFileMissing(t, config.PendingPath)
	assertFileContent(t, segmentPath, line)
}

func prepareTestPending(t *testing.T, config Config, event Event) (string, []byte, pendingOperation) {
	t.Helper()

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	line := append(encoded, '\n')
	segmentPath := SegmentPath(config.JournalRoot, event.RecordedAt)

	operation, err := preparePendingOperation(config.JournalRoot, segmentPath, line)
	if err != nil {
		t.Fatalf("prepare pending operation: %v", err)
	}

	return segmentPath, line, operation
}

func writeCompletePendingTail(t *testing.T, segmentPath string, line []byte) {
	t.Helper()

	if err := appendEventLine(segmentPath, line); err != nil {
		t.Fatalf("write complete tail: %v", err)
	}
}

func writePartialPendingTail(t *testing.T, segmentPath string, line []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(segmentPath), 0o700); err != nil {
		t.Fatalf("create segment directory: %v", err)
	}

	if err := os.WriteFile(segmentPath, line[:len(line)/2], 0o600); err != nil {
		t.Fatalf("write partial tail: %v", err)
	}
}

func writeMismatchedPendingTail(t *testing.T, segmentPath string, _ []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(segmentPath), 0o700); err != nil {
		t.Fatalf("create segment directory: %v", err)
	}

	if err := os.WriteFile(segmentPath, []byte("mismatch"), 0o600); err != nil {
		t.Fatalf("write mismatched tail: %v", err)
	}
}

func assertBlockedPendingRecovery(t *testing.T, pendingPath string, err error) {
	t.Helper()

	if !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("Load() error = %v, want ErrJournalCorrupt", err)
	}

	if _, statErr := os.Stat(pendingPath); statErr != nil {
		t.Fatalf("pending operation was not preserved: %v", statErr)
	}
}

func TestJournalAppendRecoversPriorPending(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	pendingEvent := validMilestoneEvent()

	_, _, operation := prepareTestPending(t, config, pendingEvent)
	if err := writePendingOperation(config.PendingPath, operation); err != nil {
		t.Fatalf("write pending operation: %v", err)
	}

	newEvent := distinctMilestoneEvent(t, 2)

	result, err := journal.Append(context.Background(), newEvent)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	if !result.Recorded || journal.Count() != 2 {
		t.Fatalf("Append() result = %#v, count = %d", result, journal.Count())
	}

	assertFileMissing(t, config.PendingPath)
}

func TestJournalRecoversPartialAppendAfterExistingEvent(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	first := validMilestoneEvent()
	if _, err := journal.Append(context.Background(), first); err != nil {
		t.Fatalf("append first event: %v", err)
	}

	second := distinctMilestoneEvent(t, 2)

	segmentPath, line, operation := prepareTestPending(t, config, second)
	if operation.OriginalSize == 0 {
		t.Fatal("original segment size is zero")
	}

	if err := writePendingOperation(config.PendingPath, operation); err != nil {
		t.Fatalf("write pending operation: %v", err)
	}

	file, err := os.OpenFile(segmentPath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open segment: %v", err)
	}

	if _, err := file.Write(line[:len(line)/2]); err != nil {
		t.Fatalf("write partial append: %v", closeJournalFile(file, err))
	}

	if err := closeJournalFile(file, nil); err != nil {
		t.Fatalf("close segment: %v", err)
	}

	reloaded := NewJournal(config)
	if err := reloaded.Load(context.Background()); err != nil {
		t.Fatalf("recover journal: %v", err)
	}

	if reloaded.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", reloaded.Count())
	}

	assertFileMissing(t, config.PendingPath)
}

func TestJournalLoadRejectsMalformedPendingOperation(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)
	if err := prepareJournalDirectories(config); err != nil {
		t.Fatalf("prepare journal: %v", err)
	}

	if err := os.WriteFile(config.PendingPath, []byte("{not-json}\n"), 0o600); err != nil {
		t.Fatalf("write malformed pending operation: %v", err)
	}

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("Load() error = %v, want ErrJournalCorrupt", err)
	}

	if _, err := os.Stat(config.PendingPath); err != nil {
		t.Fatalf("malformed pending operation was not preserved: %v", err)
	}
}

func TestJournalLoadRejectsUnframedSegment(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)
	if err := prepareJournalDirectories(config); err != nil {
		t.Fatalf("prepare journal: %v", err)
	}

	event := validMilestoneEvent()

	segmentPath, line, _ := prepareTestPending(t, config, event)
	if err := os.MkdirAll(filepath.Dir(segmentPath), 0o700); err != nil {
		t.Fatalf("create segment directory: %v", err)
	}

	if err := os.WriteFile(segmentPath, line[:len(line)-1], 0o600); err != nil {
		t.Fatalf("write unframed segment: %v", err)
	}

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); !errors.Is(err, ErrJournalCorrupt) {
		t.Fatalf("Load() error = %v, want ErrJournalCorrupt", err)
	}
}

func TestPendingOperationPermissions(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)
	if err := prepareJournalDirectories(config); err != nil {
		t.Fatalf("prepare journal: %v", err)
	}

	_, _, operation := prepareTestPending(t, config, validMilestoneEvent())
	if err := writePendingOperation(config.PendingPath, operation); err != nil {
		t.Fatalf("write pending operation: %v", err)
	}

	info, err := os.Stat(config.PendingPath)
	if err != nil {
		t.Fatalf("stat pending operation: %v", err)
	}

	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("pending operation mode = %o, want 600", got)
	}
}

func TestJournalConcurrentAppend(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	const eventCount = 48

	events := make([]Event, eventCount)
	for index := range events {
		events[index] = distinctMilestoneEvent(t, index+1)
	}

	errorsChannel := make(chan error, eventCount)

	var waitGroup sync.WaitGroup
	waitGroup.Add(eventCount)

	for _, event := range events {
		go func() {
			defer waitGroup.Done()

			_, err := journal.Append(context.Background(), event)
			errorsChannel <- err
		}()
	}

	waitGroup.Wait()
	close(errorsChannel)

	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("concurrent Append() error = %v", err)
		}
	}

	if journal.Count() != eventCount {
		t.Fatalf("Count() = %d, want %d", journal.Count(), eventCount)
	}

	assertFileMissing(t, config.PendingPath)

	reloaded := NewJournal(config)
	if err := reloaded.Load(context.Background()); err != nil {
		t.Fatalf("reload journal: %v", err)
	}

	if reloaded.Count() != eventCount {
		t.Fatalf("reloaded Count() = %d, want %d", reloaded.Count(), eventCount)
	}
}

func distinctMilestoneEvent(t *testing.T, sequence int) Event {
	t.Helper()

	event := validMilestoneEvent()
	event.RecordedAt = event.RecordedAt.Add(time.Duration(sequence) * time.Millisecond)
	event.Properties.ObjectID = fmt.Sprintf("%040x", sequence)

	entropy := make([]byte, 10)
	entropy[8] = byte(sequence >> 8)
	entropy[9] = byte(sequence)

	id, err := NewEventID(event.RecordedAt, bytes.NewReader(entropy))
	if err != nil {
		t.Fatalf("generate event ID: %v", err)
	}

	event.ID = id

	return event
}

func assertFileMissing(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(%q) error = %v, want os.ErrNotExist", path, err)
	}
}

func assertFileContent(t *testing.T, path string, want []byte) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

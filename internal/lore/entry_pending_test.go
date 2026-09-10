package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type entryPendingRecoveryCase struct {
	name      string
	writeTail func(*testing.T, string, []byte)
	wantErr   bool
}

func TestEntryJournalRecoversPendingStates(t *testing.T) {
	t.Parallel()

	// Recovery handles no append, a completed append, and an exact partial
	// append while preserving mismatched evidence.
	tests := []entryPendingRecoveryCase{
		{name: "missing append"},
		{name: "complete append", writeTail: writeCompleteEntryPendingTail},
		{name: "partial append", writeTail: writePartialEntryPendingTail},
		{name: "mismatched append", writeTail: writeMismatchedEntryPendingTail, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assertEntryPendingRecovery(t, test)
		})
	}
}

func TestEntryJournalAppendRecoversPriorPending(t *testing.T) {
	t.Parallel()

	// An append first completes a prior durable operation and rebuilds indexes.
	config := entryJournalConfig(t)
	journal := loadEntryJournal(t, config)

	pendingEntry := distinctKnowledgeEntry(t, 1)
	_, _, operation := prepareTestEntryPending(t, config, pendingEntry)
	writeTestEntryPending(t, config, operation)

	newEntry := distinctKnowledgeEntry(t, 2)
	if _, err := journal.Append(context.Background(), newEntry); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	if journal.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", journal.Count())
	}

	assertEntryPendingDirectoryEmpty(t, config.EntryPendingRoot)
}

func TestEntryJournalRecoversPendingAfterUnframedEntry(t *testing.T) {
	t.Parallel()

	// Pending append bytes retain the separator needed after a complete final
	// JSON object without an LF.
	config := entryJournalConfig(t)
	prepareEntryJournalForTest(t, config)

	first := distinctKnowledgeEntry(t, 1)
	writeStoredEntries(t, config, false, first)

	second := distinctKnowledgeEntry(t, 2)

	segmentPath, appendBytes, operation := prepareTestEntryPending(t, config, second)
	if len(appendBytes) == 0 || appendBytes[0] != '\n' {
		t.Fatalf("pending append = %q, want leading LF", appendBytes)
	}

	writeTestEntryPending(t, config, operation)
	writePartialEntryPendingTail(t, segmentPath, appendBytes)

	journal := loadEntryJournal(t, config)
	if journal.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", journal.Count())
	}

	assertEntryPendingDirectoryEmpty(t, config.EntryPendingRoot)
}

func TestEntryJournalRejectsAmbiguousPendingState(t *testing.T) {
	t.Parallel()

	// More than one published operation cannot be recovered deterministically.
	config := entryJournalConfig(t)
	prepareEntryJournalForTest(t, config)

	for _, entry := range []Entry{distinctKnowledgeEntry(t, 1), distinctKnowledgeEntry(t, 2)} {
		path := entryPendingFile(t, config, entry.ID)
		if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("write pending artifact: %v", err)
		}
	}

	journal := NewEntryJournal(config)
	if err := journal.Load(context.Background()); !errors.Is(err, ErrEntryJournalCorrupt) {
		t.Fatalf("Load() error = %v, want ErrEntryJournalCorrupt", err)
	}
}

func TestEntryJournalRejectsMalformedPendingState(t *testing.T) {
	t.Parallel()

	// Corrupt pending data remains present for explicit inspection.
	config := entryJournalConfig(t)
	prepareEntryJournalForTest(t, config)

	entry := distinctKnowledgeEntry(t, 1)

	pendingPath := entryPendingFile(t, config, entry.ID)
	if err := os.WriteFile(pendingPath, []byte("{not-json}\n"), 0o600); err != nil {
		t.Fatalf("write pending artifact: %v", err)
	}

	journal := NewEntryJournal(config)
	if err := journal.Load(context.Background()); !errors.Is(err, ErrEntryJournalCorrupt) {
		t.Fatalf("Load() error = %v, want ErrEntryJournalCorrupt", err)
	}

	if _, err := os.Stat(pendingPath); err != nil {
		t.Fatalf("pending artifact was not preserved: %v", err)
	}
}

func TestEntryJournalRejectsMismatchedPendingFilename(t *testing.T) {
	t.Parallel()

	// The durable filename is part of the operation identity.
	config := entryJournalConfig(t)
	prepareEntryJournalForTest(t, config)

	entry := distinctKnowledgeEntry(t, 1)
	_, _, operation := prepareTestEntryPending(t, config, entry)

	wrongID := distinctKnowledgeEntry(t, 2).ID
	wrongPath := entryPendingFile(t, config, wrongID)
	writeEncodedEntryPending(t, wrongPath, operation)

	journal := NewEntryJournal(config)
	if err := journal.Load(context.Background()); !errors.Is(err, ErrEntryJournalCorrupt) {
		t.Fatalf("Load() error = %v, want ErrEntryJournalCorrupt", err)
	}

	if _, err := os.Stat(wrongPath); err != nil {
		t.Fatalf("mismatched pending artifact was not preserved: %v", err)
	}
}

func TestEntryPendingOperationPermissions(t *testing.T) {
	t.Parallel()

	// A published pending operation is private user data.
	config := entryJournalConfig(t)
	prepareEntryJournalForTest(t, config)

	entry := distinctKnowledgeEntry(t, 1)
	_, _, operation := prepareTestEntryPending(t, config, entry)
	writeTestEntryPending(t, config, operation)

	assertEntryFileMode(t, entryPendingFile(t, config, entry.ID), 0o600)
}

func TestEntryJournalRejectsUnexpectedPendingArtifact(t *testing.T) {
	t.Parallel()

	// Recovery fails closed when the pending directory contains an artifact it
	// cannot identify as a published operation.
	config := entryJournalConfig(t)
	prepareEntryJournalForTest(t, config)

	unexpected := filepath.Join(config.EntryPendingRoot, "leftover.tmp")
	if err := os.WriteFile(unexpected, []byte("temporary"), 0o600); err != nil {
		t.Fatalf("write unexpected artifact: %v", err)
	}

	journal := NewEntryJournal(config)
	if err := journal.Load(context.Background()); !errors.Is(err, ErrEntryJournalCorrupt) {
		t.Fatalf("Load() error = %v, want ErrEntryJournalCorrupt", err)
	}
}

func assertEntryPendingRecovery(t *testing.T, test entryPendingRecoveryCase) {
	t.Helper()

	config := entryJournalConfig(t)
	prepareEntryJournalForTest(t, config)

	entry := distinctKnowledgeEntry(t, 1)
	segmentPath, appendBytes, operation := prepareTestEntryPending(t, config, entry)
	writeTestEntryPending(t, config, operation)

	if test.writeTail != nil {
		test.writeTail(t, segmentPath, appendBytes)
	}

	journal := NewEntryJournal(config)
	err := journal.Load(context.Background())

	if test.wantErr {
		if !errors.Is(err, ErrEntryJournalCorrupt) {
			t.Fatalf("Load() error = %v, want ErrEntryJournalCorrupt", err)
		}

		if _, statErr := os.Stat(entryPendingFile(t, config, entry.ID)); statErr != nil {
			t.Fatalf("pending operation was not preserved: %v", statErr)
		}

		return
	}

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if journal.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", journal.Count())
	}

	assertEntryPendingDirectoryEmpty(t, config.EntryPendingRoot)
	assertEntryFileContent(t, segmentPath, appendBytes)
}

func prepareTestEntryPending(
	t *testing.T,
	config Config,
	entry Entry,
) (string, []byte, entryPendingOperation) {
	t.Helper()

	line, err := canonicalEntryLine(entry)
	if err != nil {
		t.Fatalf("canonicalEntryLine() error = %v", err)
	}

	segmentPath := EntrySegmentPath(config.EntriesRoot, entry.RecordedAt)

	operation, err := prepareEntryPendingOperation(config.EntriesRoot, segmentPath, entry.ID, line)
	if err != nil {
		t.Fatalf("prepareEntryPendingOperation() error = %v", err)
	}

	return segmentPath, []byte(operation.Append), operation
}

func writeTestEntryPending(t *testing.T, config Config, operation entryPendingOperation) {
	t.Helper()

	pendingPath := entryPendingFile(t, config, operation.EntryID)
	if err := writeEntryPendingOperation(config.EntryPendingRoot, pendingPath, operation); err != nil {
		t.Fatalf("writeEntryPendingOperation() error = %v", err)
	}
}

func writeEncodedEntryPending(t *testing.T, path string, operation entryPendingOperation) {
	t.Helper()

	encoded, err := json.Marshal(operation)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatalf("write pending operation: %v", err)
	}
}

func writeCompleteEntryPendingTail(t *testing.T, segmentPath string, appendBytes []byte) {
	t.Helper()

	if err := appendEventLine(segmentPath, appendBytes); err != nil {
		t.Fatalf("write complete pending tail: %v", err)
	}
}

func writePartialEntryPendingTail(t *testing.T, segmentPath string, appendBytes []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(segmentPath), 0o700); err != nil {
		t.Fatalf("create segment directory: %v", err)
	}

	file, err := os.OpenFile(segmentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatalf("open segment: %v", err)
	}

	_, writeErr := file.Write(appendBytes[:len(appendBytes)/2])
	if err := errors.Join(writeErr, file.Close()); err != nil {
		t.Fatalf("write partial pending tail: %v", err)
	}
}

func writeMismatchedEntryPendingTail(t *testing.T, segmentPath string, _ []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(segmentPath), 0o700); err != nil {
		t.Fatalf("create segment directory: %v", err)
	}

	if err := os.WriteFile(segmentPath, []byte("mismatch"), 0o600); err != nil {
		t.Fatalf("write mismatched pending tail: %v", err)
	}
}

func assertEntryPendingDirectoryEmpty(t *testing.T, root string) {
	t.Helper()

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("pending directory contains %#v", entries)
	}
}

func assertEntryFileContent(t *testing.T, path string, want []byte) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("content = %q, want %q", got, want)
	}
}

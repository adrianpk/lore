package lore

import (
	"bufio"
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

func TestEntrySegmentPath(t *testing.T) {
	t.Parallel()

	// Segment selection always uses the UTC recording month.
	timestamp := time.Date(2026, 8, 31, 23, 30, 0, 0, time.FixedZone("offset", -2*60*60))
	got := EntrySegmentPath("/entries", timestamp)

	want := filepath.FromSlash("/entries/2026/09.jsonl")
	if got != want {
		t.Fatalf("EntrySegmentPath() = %q, want %q", got, want)
	}
}

func TestEntryJournalAppendLoadAndCanonicalize(t *testing.T) {
	t.Parallel()

	// A root entry is normalized once, persisted canonically, and reloads with
	// the same identity and content.
	config := entryJournalConfig(t)
	journal := loadEntryJournal(t, config)

	entry := distinctKnowledgeEntry(t, 1)
	entry.Contexts[0].Name = " Lore Project "
	entry.Labels[0] = " Service Recovery "

	result, err := journal.Append(context.Background(), entry)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	if result.Entry.Contexts[0].Name != "lore-project" || result.Entry.Labels[0] != "service-recovery" {
		t.Fatalf("Append() entry = %#v", result.Entry)
	}

	if journal.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", journal.Count())
	}

	segmentPath := EntrySegmentPath(config.EntriesRoot, entry.RecordedAt)
	assertEntryFileMode(t, segmentPath, 0o600)
	assertEntryFileMode(t, filepath.Dir(segmentPath), 0o700)
	assertEntryFileMode(t, config.EntryPendingRoot, 0o700)

	reloaded := loadEntryJournal(t, config)

	got, exists := reloaded.Entry(entry.ID)
	if !exists || got.ID != entry.ID || got.Contexts[0].Name != "lore-project" {
		t.Fatalf("Entry() = %#v, %v", got, exists)
	}
}

func TestEntryJournalAppendRequiresLoad(t *testing.T) {
	t.Parallel()

	// Callers cannot publish before recovery and index reconstruction complete.
	journal := NewEntryJournal(entryJournalConfig(t))

	_, err := journal.Append(context.Background(), distinctKnowledgeEntry(t, 1))
	if !errors.Is(err, ErrEntryJournalNotLoaded) {
		t.Fatalf("Append() error = %v, want ErrEntryJournalNotLoaded", err)
	}
}

func TestEntryJournalReadIsolation(t *testing.T) {
	t.Parallel()

	// Returned entries cannot mutate the disposable in-memory source of truth.
	journal := loadEntryJournal(t, entryJournalConfig(t))
	entry := distinctKnowledgeEntry(t, 1)

	result, err := journal.Append(context.Background(), entry)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	result.Entry.Contexts[0].Name = "changed"
	result.Entry.Labels[0] = "changed"
	result.Entry.References[0].Value = "changed"
	result.Entry.Provenance.Harness = "changed"

	stored, exists := journal.Entry(entry.ID)
	if !exists {
		t.Fatal("Entry() did not find stored entry")
	}

	mutated := stored.Contexts[0].Name == "changed" || stored.Labels[0] == "changed" ||
		stored.References[0].Value == "changed" || stored.Provenance.Harness == "changed"
	if mutated {
		t.Fatalf("stored entry was mutated: %#v", stored)
	}

	stored.Contexts[0].Name = "changed-again"

	effective, exists := journal.Effective(entry.ID)
	if !exists || effective.Contexts[0].Name == "changed-again" {
		t.Fatalf("Effective() leaked caller mutation: %#v, %v", effective, exists)
	}
}

func TestEntryJournalMonthlySegments(t *testing.T) {
	t.Parallel()

	// Recording time, not process-local time, owns monthly placement.
	config := entryJournalConfig(t)
	journal := loadEntryJournal(t, config)

	august := distinctKnowledgeEntry(t, 1)
	august.RecordedAt = time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC)
	september := distinctKnowledgeEntry(t, 2)
	september.RecordedAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	for _, entry := range []Entry{august, september} {
		if _, err := journal.Append(context.Background(), entry); err != nil {
			t.Fatalf("Append(%s) error = %v", entry.ID, err)
		}

		if _, err := os.Stat(EntrySegmentPath(config.EntriesRoot, entry.RecordedAt)); err != nil {
			t.Fatalf("stat segment for %s: %v", entry.ID, err)
		}
	}

	if journal.Count() != 2 {
		t.Fatalf("Count() = %d, want 2", journal.Count())
	}
}

func TestEntryJournalRevisionChain(t *testing.T) {
	t.Parallel()

	// Every chain member resolves to the terminal revision while history stays
	// complete and ordered.
	config := entryJournalConfig(t)
	journal := loadEntryJournal(t, config)

	root := distinctKnowledgeEntry(t, 1)
	root.RecordedAt = time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC)
	appendKnowledgeEntry(t, journal, root)

	archived := entryRevision(t, root, 2, EntryStatusArchived)
	appendKnowledgeEntry(t, journal, archived)

	restored := entryRevision(t, archived, 3, EntryStatusActive)
	appendKnowledgeEntry(t, journal, restored)

	assertEffectiveRevision(t, journal, restored.ID, root.ID, archived.ID, restored.ID)
	assertEntryHistory(t, journal, archived.ID, root.ID, archived.ID, restored.ID)

	reloaded := loadEntryJournal(t, config)

	effective, exists := reloaded.Effective(root.ID)
	if !exists || effective.ID != restored.ID {
		t.Fatalf("reloaded Effective() = %#v, %v", effective, exists)
	}
}

func TestEntryJournalRejectsInvalidRevisions(t *testing.T) {
	t.Parallel()

	// Persistence owns predecessor existence, effective-head, kind, and
	// chronology invariants.
	tests := []struct {
		name    string
		prepare func(*testing.T, *EntryJournal) Entry
		wantErr error
	}{
		{name: "missing predecessor", prepare: missingPredecessorRevision, wantErr: ErrEntryRevisionTargetMissing},
		{name: "stale predecessor", prepare: stalePredecessorRevision, wantErr: ErrEntryRevisionTargetStale},
		{name: "kind change", prepare: changedKindRevision},
		{name: "backdated", prepare: backdatedRevision},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			journal := loadEntryJournal(t, entryJournalConfig(t))
			entry := test.prepare(t, journal)

			_, err := journal.Append(context.Background(), entry)
			if err == nil {
				t.Fatal("expected an error")
			}

			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("Append() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestEntryJournalLoadRejectsCorruption(t *testing.T) {
	t.Parallel()

	// Invalid placement, encoding, duplicate identity, and broken chains fail
	// the complete load instead of yielding a partial index.
	tests := []struct {
		name  string
		write func(*testing.T, Config)
	}{
		{name: "wrong segment", write: writeEntryToWrongSegment},
		{name: "unknown field", write: writeEntryWithUnknownField},
		{name: "noncanonical JSON", write: writeNoncanonicalEntryJSON},
		{name: "duplicate ID", write: writeDuplicateEntryID},
		{name: "missing predecessor", write: writeMissingPredecessor},
		{name: "fork", write: writeForkedHistory},
		{name: "cycle", write: writeCyclicHistory},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			config := entryJournalConfig(t)
			prepareEntryJournalForTest(t, config)
			test.write(t, config)

			journal := NewEntryJournal(config)
			if err := journal.Load(context.Background()); !errors.Is(err, ErrEntryJournalCorrupt) {
				t.Fatalf("Load() error = %v, want ErrEntryJournalCorrupt", err)
			}

			if journal.Loaded() || journal.Count() != 0 {
				t.Fatalf("failed journal state = loaded %v, count %d", journal.Loaded(), journal.Count())
			}
		})
	}
}

func TestEntryJournalToleratesMissingFinalNewline(t *testing.T) {
	t.Parallel()

	// A complete final JSON object remains readable and the next append repairs
	// JSONL framing with exactly one separator.
	config := entryJournalConfig(t)
	prepareEntryJournalForTest(t, config)

	first := distinctKnowledgeEntry(t, 1)
	writeStoredEntries(t, config, false, first)

	journal := loadEntryJournal(t, config)
	second := distinctKnowledgeEntry(t, 2)
	appendKnowledgeEntry(t, journal, second)

	segmentPath := EntrySegmentPath(config.EntriesRoot, first.RecordedAt)
	if got := countEntryLines(t, segmentPath); got != 2 {
		t.Fatalf("segment lines = %d, want 2", got)
	}

	content, err := os.ReadFile(segmentPath)
	if err != nil {
		t.Fatalf("read segment: %v", err)
	}

	if bytes.Contains(content, []byte("\n\n")) || content[len(content)-1] != '\n' {
		t.Fatalf("repaired segment framing = %q", content)
	}
}

func TestEntryJournalConcurrentAppend(t *testing.T) {
	t.Parallel()

	// The in-process journal serializes append and pending-state transitions.
	config := entryJournalConfig(t)
	journal := loadEntryJournal(t, config)

	const entryCount = 32

	errorsChannel := make(chan error, entryCount)

	var waitGroup sync.WaitGroup
	waitGroup.Add(entryCount)

	for sequence := 1; sequence <= entryCount; sequence++ {
		entry := distinctKnowledgeEntry(t, sequence)

		go func() {
			defer waitGroup.Done()

			_, err := journal.Append(context.Background(), entry)
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

	if journal.Count() != entryCount {
		t.Fatalf("Count() = %d, want %d", journal.Count(), entryCount)
	}

	assertEntryFileMissing(t, entryPendingFile(t, config, distinctKnowledgeEntry(t, 1).ID))

	reloaded := loadEntryJournal(t, config)
	if reloaded.Count() != entryCount {
		t.Fatalf("reloaded Count() = %d, want %d", reloaded.Count(), entryCount)
	}
}

func TestEntryJournalLoadHonorsCancellation(t *testing.T) {
	t.Parallel()

	// Cancellation prevents directory or index mutation before load starts.
	config := entryJournalConfig(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	journal := NewEntryJournal(config)
	if err := journal.Load(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load() error = %v, want context.Canceled", err)
	}

	if _, err := os.Stat(config.DataRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(DataRoot) error = %v, want os.ErrNotExist", err)
	}
}

func entryJournalConfig(t *testing.T) Config {
	t.Helper()

	config, err := ConfigFromHome(t.TempDir())
	if err != nil {
		t.Fatalf("ConfigFromHome() error = %v", err)
	}

	return config
}

func loadEntryJournal(t *testing.T, config Config) *EntryJournal {
	t.Helper()

	journal := NewEntryJournal(config)
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	return journal
}

func appendKnowledgeEntry(t *testing.T, journal *EntryJournal, entry Entry) Entry {
	t.Helper()

	result, err := journal.Append(context.Background(), entry)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	return result.Entry
}

func distinctKnowledgeEntry(t *testing.T, sequence int) Entry {
	t.Helper()

	entry := validKnowledgeEntry()
	entry.RecordedAt = entry.RecordedAt.Add(time.Duration(sequence) * time.Millisecond)
	entry.VerifiedAt = nil
	entry.Title = fmt.Sprintf("Knowledge entry %d", sequence)
	entry.Body = fmt.Sprintf("Reusable operational knowledge %d.", sequence)

	entropy := make([]byte, 10)
	entropy[8] = byte(sequence >> 8)
	entropy[9] = byte(sequence)

	id, err := NewEntryID(entry.RecordedAt, bytes.NewReader(entropy))
	if err != nil {
		t.Fatalf("NewEntryID() error = %v", err)
	}

	entry.ID = id

	return entry
}

func entryRevision(t *testing.T, previous Entry, sequence int, status EntryStatus) Entry {
	t.Helper()

	replacement := previous
	replacement.Status = status
	replacement.VerifiedAt = nil

	next := distinctKnowledgeEntry(t, sequence)

	revision, err := NewEntryRevision(previous, replacement, next.ID, next.RecordedAt, "Lifecycle update.")
	if err != nil {
		t.Fatalf("NewEntryRevision() error = %v", err)
	}

	return revision
}

func missingPredecessorRevision(t *testing.T, _ *EntryJournal) Entry {
	t.Helper()

	entry := distinctKnowledgeEntry(t, 2)
	entry.Supersedes = distinctKnowledgeEntry(t, 1).ID
	entry.RevisionReason = "Missing target."

	return entry
}

func stalePredecessorRevision(t *testing.T, journal *EntryJournal) Entry {
	t.Helper()

	root := distinctKnowledgeEntry(t, 1)
	appendKnowledgeEntry(t, journal, root)
	appendKnowledgeEntry(t, journal, entryRevision(t, root, 2, EntryStatusArchived))

	stale := entryRevision(t, root, 3, EntryStatusInvalidated)

	return stale
}

func changedKindRevision(t *testing.T, journal *EntryJournal) Entry {
	t.Helper()

	root := distinctKnowledgeEntry(t, 1)
	appendKnowledgeEntry(t, journal, root)

	revision := distinctKnowledgeEntry(t, 2)
	revision.Kind = EntryKindWarning
	revision.Supersedes = root.ID
	revision.RevisionReason = "Change kind."

	return revision
}

func backdatedRevision(t *testing.T, journal *EntryJournal) Entry {
	t.Helper()

	root := distinctKnowledgeEntry(t, 2)
	appendKnowledgeEntry(t, journal, root)

	revision := distinctKnowledgeEntry(t, 1)
	revision.Supersedes = root.ID
	revision.RevisionReason = "Backdated revision."

	return revision
}

func prepareEntryJournalForTest(t *testing.T, config Config) {
	t.Helper()

	if err := prepareEntryDirectories(config); err != nil {
		t.Fatalf("prepare entry journal: %v", err)
	}
}

func writeStoredEntries(t *testing.T, config Config, trailingNewline bool, entries ...Entry) {
	t.Helper()

	if len(entries) == 0 {
		t.Fatal("writeStoredEntries() requires entries")
	}

	segmentPath := EntrySegmentPath(config.EntriesRoot, entries[0].RecordedAt)
	if err := os.MkdirAll(filepath.Dir(segmentPath), 0o700); err != nil {
		t.Fatalf("create segment directory: %v", err)
	}

	var content []byte

	for index, entry := range entries {
		if EntrySegmentPath(config.EntriesRoot, entry.RecordedAt) != segmentPath {
			t.Fatal("entries belong to different segments")
		}

		line, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}

		if index > 0 {
			content = append(content, '\n')
		}

		content = append(content, line...)
	}

	if trailingNewline {
		content = append(content, '\n')
	}

	if err := os.WriteFile(segmentPath, content, 0o600); err != nil {
		t.Fatalf("write segment: %v", err)
	}
}

func writeEntryToWrongSegment(t *testing.T, config Config) {
	t.Helper()

	entry := distinctKnowledgeEntry(t, 1)
	path := filepath.Join(config.EntriesRoot, "2026", "10.jsonl")
	writeEntryBytes(t, path, mustEntryJSON(t, entry))
}

func writeEntryWithUnknownField(t *testing.T, config Config) {
	t.Helper()

	entry := distinctKnowledgeEntry(t, 1)
	encoded := mustEntryJSON(t, entry)
	encoded = append(encoded[:len(encoded)-1], []byte(`,"unknown":true}`)...)
	writeEntryBytes(t, EntrySegmentPath(config.EntriesRoot, entry.RecordedAt), encoded)
}

func writeNoncanonicalEntryJSON(t *testing.T, config Config) {
	t.Helper()

	entry := distinctKnowledgeEntry(t, 1)
	encoded := append([]byte{' '}, mustEntryJSON(t, entry)...)
	writeEntryBytes(t, EntrySegmentPath(config.EntriesRoot, entry.RecordedAt), encoded)
}

func writeDuplicateEntryID(t *testing.T, config Config) {
	t.Helper()

	entry := distinctKnowledgeEntry(t, 1)
	writeStoredEntries(t, config, true, entry, entry)
}

func writeMissingPredecessor(t *testing.T, config Config) {
	t.Helper()

	revision := missingPredecessorRevision(t, nil)
	writeStoredEntries(t, config, true, revision)
}

func writeForkedHistory(t *testing.T, config Config) {
	t.Helper()

	root := distinctKnowledgeEntry(t, 1)
	first := entryRevision(t, root, 2, EntryStatusArchived)
	second := entryRevision(t, root, 3, EntryStatusInvalidated)
	writeStoredEntries(t, config, true, root, first, second)
}

func writeCyclicHistory(t *testing.T, config Config) {
	t.Helper()

	first := distinctKnowledgeEntry(t, 1)
	second := distinctKnowledgeEntry(t, 2)
	first.Supersedes = second.ID
	first.RevisionReason = "Cycle first."
	second.Supersedes = first.ID
	second.RevisionReason = "Cycle second."
	writeStoredEntries(t, config, true, first, second)
}

func writeEntryBytes(t *testing.T, path string, encoded []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create segment directory: %v", err)
	}

	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		t.Fatalf("write segment: %v", err)
	}
}

func mustEntryJSON(t *testing.T, entry Entry) []byte {
	t.Helper()

	encoded, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	return encoded
}

func countEntryLines(t *testing.T, path string) int {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open entry segment: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close entry segment: %v", err)
		}
	}()

	count := 0

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan entry segment: %v", err)
	}

	return count
}

func assertEntryFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}

	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode %q = %o, want %o", path, got, want)
	}
}

func assertEntryFileMissing(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat(%q) error = %v, want os.ErrNotExist", path, err)
	}
}

func entryPendingFile(t *testing.T, config Config, id EntryID) string {
	t.Helper()

	return filepath.Join(config.EntryPendingRoot, string(id)+".json")
}

func assertEffectiveRevision(t *testing.T, journal *EntryJournal, want EntryID, ids ...EntryID) {
	t.Helper()

	for _, id := range ids {
		effective, exists := journal.Effective(id)
		if !exists || effective.ID != want {
			t.Fatalf("Effective(%s) = %#v, %v", id, effective, exists)
		}
	}

	effectiveEntries := journal.EffectiveEntries()
	if len(effectiveEntries) != 1 || effectiveEntries[0].ID != want {
		t.Fatalf("EffectiveEntries() = %#v", effectiveEntries)
	}
}

func assertEntryHistory(t *testing.T, journal *EntryJournal, id EntryID, want ...EntryID) {
	t.Helper()

	history, exists := journal.History(id)
	if !exists || len(history) != len(want) {
		t.Fatalf("History() = %#v, %v", history, exists)
	}

	for index, entry := range history {
		if entry.ID != want[index] {
			t.Fatalf("History()[%d].ID = %s, want %s", index, entry.ID, want[index])
		}
	}
}

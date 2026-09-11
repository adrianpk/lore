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
	"time"
)

var (
	// ErrEntryJournalNotLoaded indicates that entry IO started before Load.
	ErrEntryJournalNotLoaded = errors.New("entry journal is not loaded")
	// ErrEntryJournalCorrupt indicates invalid canonical entry-journal content.
	ErrEntryJournalCorrupt = errors.New("entry journal is corrupt")
)

// EntryAppendResult reports the entry durably appended by EntryJournal.Append.
type EntryAppendResult struct {
	Entry Entry `json:"entry"`
}

// EntryJournal owns canonical monthly knowledge segments and disposable
// indexes.
type EntryJournal struct {
	config Config

	mu     sync.RWMutex
	loaded bool
	index  *entryIndex
}

// NewEntryJournal stores configuration without performing IO.
func NewEntryJournal(config Config) *EntryJournal {
	return &EntryJournal{config: config, index: newEntryIndex()}
}

// Load creates Lore-owned entry directories, recovers one pending append, and
// rebuilds the entry index.
func (journal *EntryJournal) Load(ctx context.Context) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()

	journal.loaded = false
	journal.index = newEntryIndex()

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := prepareEntryBaseDirectories(journal.config); err != nil {
		return err
	}

	if err := recoverEntryPurge(journal.config); err != nil {
		return err
	}

	if err := prepareEntrySegmentDirectory(journal.config.EntriesRoot); err != nil {
		return err
	}

	if _, err := recoverEntryPending(journal.config); err != nil {
		return err
	}

	index, err := rebuildEntryIndex(ctx, journal.config.EntriesRoot)
	if err != nil {
		return err
	}

	journal.index = index
	journal.loaded = true

	return nil
}

// Loaded reports whether canonical entry indexes are available.
func (journal *EntryJournal) Loaded() bool {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	return journal.loaded
}

// Append validates and durably publishes one knowledge revision.
func (journal *EntryJournal) Append(ctx context.Context, entry Entry) (EntryAppendResult, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()

	if !journal.loaded {
		return EntryAppendResult{}, ErrEntryJournalNotLoaded
	}

	if err := journal.recoverForAppend(ctx); err != nil {
		return EntryAppendResult{}, err
	}

	if err := ctx.Err(); err != nil {
		return EntryAppendResult{}, err
	}

	canonical, err := entry.Canonicalize()
	if err != nil {
		return EntryAppendResult{}, err
	}

	if err := validateEntryAgainstIndex(canonical, journal.index); err != nil {
		return EntryAppendResult{}, err
	}

	line, err := canonicalEntryLine(canonical)
	if err != nil {
		return EntryAppendResult{}, err
	}

	segmentPath := EntrySegmentPath(journal.config.EntriesRoot, canonical.RecordedAt)
	if err := publishEntry(journal.config, segmentPath, canonical.ID, line); err != nil {
		return EntryAppendResult{}, err
	}

	addEntryToIndex(canonical, journal.index)

	return EntryAppendResult{Entry: cloneEntry(canonical)}, nil
}

// Entry returns one raw entry revision by ID.
func (journal *EntryJournal) Entry(id EntryID) (Entry, bool) {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	entry, exists := journal.index.entries[id]

	return cloneEntry(entry), exists
}

// Count returns the number of canonical entry revisions loaded in memory.
func (journal *EntryJournal) Count() int {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	return len(journal.index.entries)
}

// Effective returns the terminal effective revision for any chain member.
func (journal *EntryJournal) Effective(id EntryID) (Entry, bool) {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	root, exists := journal.index.roots[id]
	if !exists {
		return Entry{}, false
	}

	effectiveID, exists := journal.index.effective[root]
	if !exists {
		return Entry{}, false
	}

	return cloneEntry(journal.index.entries[effectiveID]), true
}

// History returns one complete revision chain in oldest-to-newest order.
func (journal *EntryJournal) History(id EntryID) ([]Entry, bool) {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	root, exists := journal.index.roots[id]
	if !exists {
		return nil, false
	}

	history := make([]Entry, 0)
	for current := root; current != ""; current = journal.index.successors[current] {
		history = append(history, cloneEntry(journal.index.entries[current]))
	}

	return history, true
}

// EffectiveEntries returns every terminal revision in recording order.
func (journal *EntryJournal) EffectiveEntries() []Entry {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	entries := make([]Entry, 0, len(journal.index.effective))
	for _, id := range journal.index.order {
		root := journal.index.roots[id]
		if journal.index.effective[root] == id {
			entries = append(entries, cloneEntry(journal.index.entries[id]))
		}
	}

	return entries
}

// EntrySegmentPath returns the canonical monthly entry path for a recording
// time.
func EntrySegmentPath(root string, recordedAt time.Time) string {
	utc := recordedAt.UTC()

	return filepath.Join(root, fmt.Sprintf("%04d", utc.Year()), fmt.Sprintf("%02d.jsonl", int(utc.Month())))
}

func (journal *EntryJournal) recoverForAppend(ctx context.Context) error {
	recovered, err := recoverEntryPending(journal.config)
	if err != nil || !recovered {
		return err
	}

	index, err := rebuildEntryIndex(ctx, journal.config.EntriesRoot)
	if err != nil {
		return err
	}

	journal.index = index

	return nil
}

func prepareEntryBaseDirectories(config Config) error {
	for _, directory := range []string{config.DataRoot, config.EntryPendingRoot} {
		if directory == "" {
			return fmt.Errorf("prepare entry journal: configured path is empty")
		}

		if err := os.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("prepare entry directory %q: %w", directory, err)
		}

		if err := os.Chmod(directory, 0o700); err != nil {
			return fmt.Errorf("set entry directory permissions %q: %w", directory, err)
		}
	}

	return nil
}

func prepareEntryDirectories(config Config) error {
	if err := prepareEntryBaseDirectories(config); err != nil {
		return err
	}

	return prepareEntrySegmentDirectory(config.EntriesRoot)
}

func prepareEntrySegmentDirectory(directory string) error {
	if directory == "" {
		return fmt.Errorf("prepare entry journal: configured path is empty")
	}

	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("prepare entry directory %q: %w", directory, err)
	}

	if err := os.Chmod(directory, 0o700); err != nil {
		return fmt.Errorf("set entry directory permissions %q: %w", directory, err)
	}

	return nil
}

func rebuildEntryIndex(ctx context.Context, root string) (*entryIndex, error) {
	paths, err := listSegmentPaths(root)
	if err != nil {
		return nil, err
	}

	index := newEntryIndex()

	for _, segmentPath := range paths {
		if err := loadEntrySegment(ctx, root, segmentPath, index); err != nil {
			return nil, err
		}
	}

	return index, nil
}

func loadEntrySegment(ctx context.Context, root, segmentPath string, index *entryIndex) (resultErr error) {
	file, err := os.Open(segmentPath)
	if err != nil {
		return fmt.Errorf("open entry segment %q: %w", segmentPath, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && resultErr == nil {
			resultErr = fmt.Errorf("close entry segment %q: %w", segmentPath, closeErr)
		}
	}()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxEventLineSize)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++

		if err := ctx.Err(); err != nil {
			return err
		}

		entry, err := decodeStoredEntry(root, segmentPath, lineNumber, scanner.Bytes())
		if err != nil {
			return err
		}

		if err := validateEntryAgainstIndex(entry, index); err != nil {
			return corruptEntrySegment(segmentPath, lineNumber, "%v", err)
		}

		addEntryToIndex(entry, index)
	}

	if err := scanner.Err(); err != nil {
		return corruptEntrySegment(segmentPath, lineNumber+1, "read entry: %v", err)
	}

	return nil
}

func decodeStoredEntry(root, segmentPath string, lineNumber int, line []byte) (Entry, error) {
	if len(line) == 0 {
		return Entry{}, corruptEntrySegment(segmentPath, lineNumber, "blank line")
	}

	entry, err := decodeCanonicalEntry(line)
	if err != nil {
		return Entry{}, corruptEntrySegment(segmentPath, lineNumber, "%v", err)
	}

	if EntrySegmentPath(root, entry.RecordedAt) != segmentPath {
		return Entry{}, corruptEntrySegment(
			segmentPath,
			lineNumber,
			"entry belongs to %q",
			EntrySegmentPath(root, entry.RecordedAt),
		)
	}

	canonical, err := canonicalEntryLine(entry)
	if err != nil {
		return Entry{}, corruptEntrySegment(segmentPath, lineNumber, "%v", err)
	}

	if !bytes.Equal(line, canonical[:len(canonical)-1]) {
		return Entry{}, corruptEntrySegment(segmentPath, lineNumber, "entry is not canonical JSON")
	}

	return entry, nil
}

func decodeCanonicalEntry(line []byte) (Entry, error) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()

	var entry Entry
	if err := decoder.Decode(&entry); err != nil {
		return Entry{}, fmt.Errorf("decode entry: %w", err)
	}

	if err := requireJSONEnd(decoder); err != nil {
		return Entry{}, fmt.Errorf("decode entry: %w", err)
	}

	if err := entry.Validate(); err != nil {
		return Entry{}, err
	}

	return entry, nil
}

func canonicalEntryLine(entry Entry) ([]byte, error) {
	line, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("encode entry: %w", err)
	}

	return append(line, '\n'), nil
}

func cloneEntry(entry Entry) Entry {
	entry.Contexts = append([]Context(nil), entry.Contexts...)
	entry.Labels = append([]string(nil), entry.Labels...)
	entry.References = append([]EntryReference(nil), entry.References...)

	if entry.VerifiedAt != nil {
		verifiedAt := *entry.VerifiedAt
		entry.VerifiedAt = &verifiedAt
	}

	if entry.Provenance != nil {
		provenance := *entry.Provenance
		entry.Provenance = &provenance
	}

	return entry
}

func corruptEntrySegment(segmentPath string, line int, format string, arguments ...any) error {
	message := fmt.Sprintf(format, arguments...)

	return fmt.Errorf("%w: %s:%d: %s", ErrEntryJournalCorrupt, segmentPath, line, message)
}

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

	mu      sync.RWMutex
	loaded  bool
	entries map[EntryID]Entry
	order   []EntryID
}

// NewEntryJournal stores configuration without performing IO.
func NewEntryJournal(config Config) *EntryJournal {
	return &EntryJournal{config: config}
}

// Load creates Lore-owned entry directories, recovers one pending append, and
// rebuilds the entry index.
func (journal *EntryJournal) Load(ctx context.Context) error {
	journal.mu.Lock()
	defer journal.mu.Unlock()

	journal.loaded = false
	journal.entries = make(map[EntryID]Entry)
	journal.order = nil

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := prepareEntryDirectories(journal.config); err != nil {
		return err
	}

	if _, err := recoverEntryPending(journal.config); err != nil {
		return err
	}

	entries, order, err := rebuildEntryIndex(ctx, journal.config.EntriesRoot)
	if err != nil {
		return err
	}

	journal.entries = entries
	journal.order = order
	journal.loaded = true

	return nil
}

// Loaded reports whether canonical entry indexes are available.
func (journal *EntryJournal) Loaded() bool {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	return journal.loaded
}

// Append validates and durably publishes one root knowledge entry.
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

	if canonical.Supersedes != "" {
		return EntryAppendResult{}, fmt.Errorf("append entry: revision persistence is not available")
	}

	if _, exists := journal.entries[canonical.ID]; exists {
		return EntryAppendResult{}, fmt.Errorf("append entry: duplicate entry ID %q", canonical.ID)
	}

	line, err := canonicalEntryLine(canonical)
	if err != nil {
		return EntryAppendResult{}, err
	}

	segmentPath := EntrySegmentPath(journal.config.EntriesRoot, canonical.RecordedAt)
	if err := publishEntry(journal.config, segmentPath, canonical.ID, line); err != nil {
		return EntryAppendResult{}, err
	}

	journal.entries[canonical.ID] = canonical
	journal.order = append(journal.order, canonical.ID)

	return EntryAppendResult{Entry: canonical}, nil
}

// Entry returns one raw entry revision by ID.
func (journal *EntryJournal) Entry(id EntryID) (Entry, bool) {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	entry, exists := journal.entries[id]

	return entry, exists
}

// Count returns the number of canonical entry revisions loaded in memory.
func (journal *EntryJournal) Count() int {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	return len(journal.entries)
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

	entries, order, err := rebuildEntryIndex(ctx, journal.config.EntriesRoot)
	if err != nil {
		return err
	}

	journal.entries = entries
	journal.order = order

	return nil
}

func prepareEntryDirectories(config Config) error {
	for _, directory := range []string{config.DataRoot, config.EntriesRoot, config.EntryPendingRoot} {
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

func rebuildEntryIndex(ctx context.Context, root string) (map[EntryID]Entry, []EntryID, error) {
	paths, err := listSegmentPaths(root)
	if err != nil {
		return nil, nil, err
	}

	entries := make(map[EntryID]Entry)
	order := make([]EntryID, 0)

	for _, segmentPath := range paths {
		if err := loadEntrySegment(ctx, root, segmentPath, entries, &order); err != nil {
			return nil, nil, err
		}
	}

	return entries, order, nil
}

func loadEntrySegment(
	ctx context.Context,
	root string,
	segmentPath string,
	entries map[EntryID]Entry,
	order *[]EntryID,
) (resultErr error) {
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

		if _, exists := entries[entry.ID]; exists {
			return corruptEntrySegment(segmentPath, lineNumber, "duplicate entry ID %q", entry.ID)
		}

		if entry.Supersedes != "" {
			return corruptEntrySegment(segmentPath, lineNumber, "revision persistence is not available")
		}

		entries[entry.ID] = entry
		*order = append(*order, entry.ID)
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

func corruptEntrySegment(segmentPath string, line int, format string, arguments ...any) error {
	message := fmt.Sprintf(format, arguments...)

	return fmt.Errorf("%w: %s:%d: %s", ErrEntryJournalCorrupt, segmentPath, line, message)
}

package lore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxEventLineSize = 8 << 20

var (
	// ErrJournalNotLoaded indicates that journal IO started before Load.
	ErrJournalNotLoaded = errors.New("journal is not loaded")
	// ErrJournalCorrupt indicates invalid canonical journal content.
	ErrJournalCorrupt = errors.New("journal is corrupt")
)

type naturalKey struct {
	Type          ActivityType
	ScopeIdentity string
	Value         string
}

// AppendResult reports whether Append published a new event.
type AppendResult struct {
	Event    Activity
	Recorded bool
}

// Journal owns monthly canonical segments and disposable indexes.
type Journal struct {
	config Config

	mu        sync.RWMutex
	loaded    bool
	loadErr   error
	events    map[EventID]Activity
	roots     map[EventID]EventID
	effective map[EventID]EventID
	natural   map[naturalKey]EventID
	order     []EventID
}

// NewJournal stores configuration without performing IO.
func NewJournal(config Config) *Journal {
	return &Journal{config: config}
}

// Load creates Lore-owned directories and rebuilds all journal indexes.
func (journal *Journal) Load(ctx context.Context) (resultErr error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	defer func() {
		journal.loadErr = resultErr
	}()

	journal.loaded = false
	journal.applyIndex(newJournalIndex())

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := prepareJournalDirectories(journal.config); err != nil {
		return err
	}

	if _, err := recoverPending(journal.config); err != nil {
		return err
	}

	index, err := rebuildJournalIndex(ctx, journal.config.JournalRoot)
	if err != nil {
		return err
	}

	journal.applyIndex(index)

	journal.loaded = true

	return nil
}

// Loaded reports whether canonical indexes are available for normal operations.
func (journal *Journal) Loaded() bool {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	return journal.loaded
}

// Append validates, publishes, syncs, and indexes one event.
func (journal *Journal) Append(ctx context.Context, event Activity) (AppendResult, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()

	if !journal.loaded {
		return AppendResult{}, ErrJournalNotLoaded
	}

	if err := journal.recoverForAppend(ctx); err != nil {
		return AppendResult{}, err
	}

	if err := ctx.Err(); err != nil {
		return AppendResult{}, err
	}

	if err := event.Validate(); err != nil {
		return AppendResult{}, err
	}

	if existing, ok := journal.existingNaturalEvent(event); ok {
		return AppendResult{Event: existing, Recorded: false}, nil
	}

	index := journal.indexView()
	if err := validateActivityAgainstIndex(event, index); err != nil {
		return AppendResult{}, err
	}

	line, err := canonicalActivityLine(event)
	if err != nil {
		return AppendResult{}, err
	}

	segmentPath := SegmentPath(journal.config.JournalRoot, event.RecordedAt)
	if err := publishEvent(journal.config, segmentPath, line); err != nil {
		return AppendResult{}, err
	}

	addActivityToIndex(event, index)
	journal.applyIndex(index)

	return AppendResult{Event: event, Recorded: true}, nil
}

func (journal *Journal) existingNaturalEvent(event Activity) (Activity, bool) {
	if event.Supersedes != "" {
		return Activity{}, false
	}

	key, ok := activityNaturalKey(event)
	if !ok {
		return Activity{}, false
	}

	root, ok := journal.natural[key]
	if !ok {
		return Activity{}, false
	}

	effective := journal.effective[root]

	return journal.events[effective], true
}

func (journal *Journal) recoverForAppend(ctx context.Context) error {
	recovered, err := recoverPending(journal.config)
	if err != nil || !recovered {
		return err
	}

	index, err := rebuildJournalIndex(ctx, journal.config.JournalRoot)
	if err != nil {
		return err
	}

	journal.applyIndex(index)

	return nil
}

// Event returns one raw event by ID.
func (journal *Journal) Event(id EventID) (Activity, bool) {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	event, ok := journal.events[id]

	return event, ok
}

// Effective returns the current effective event for the supplied chain member.
func (journal *Journal) Effective(id EventID) (Activity, bool) {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	root, ok := journal.roots[id]
	if !ok {
		return Activity{}, false
	}

	effectiveID, ok := journal.effective[root]
	if !ok {
		return Activity{}, false
	}

	return journal.events[effectiveID], true
}

// Count returns the number of canonical events loaded in memory.
func (journal *Journal) Count() int {
	journal.mu.RLock()
	defer journal.mu.RUnlock()

	return len(journal.events)
}

// SegmentPath returns the canonical monthly path for a recording time.
func SegmentPath(root string, recordedAt time.Time) string {
	utc := recordedAt.UTC()

	return filepath.Join(root, fmt.Sprintf("%04d", utc.Year()), fmt.Sprintf("%02d.jsonl", int(utc.Month())))
}

type journalIndex struct {
	events    map[EventID]Activity
	roots     map[EventID]EventID
	effective map[EventID]EventID
	natural   map[naturalKey]EventID
	order     []EventID
}

func newJournalIndex() *journalIndex {
	return &journalIndex{
		events:    make(map[EventID]Activity),
		roots:     make(map[EventID]EventID),
		effective: make(map[EventID]EventID),
		natural:   make(map[naturalKey]EventID),
		order:     make([]EventID, 0),
	}
}

func rebuildJournalIndex(ctx context.Context, root string) (*journalIndex, error) {
	paths, err := listSegmentPaths(root)
	if err != nil {
		return nil, err
	}

	index := newJournalIndex()
	for _, segmentPath := range paths {
		if err := loadSegment(ctx, root, segmentPath, index); err != nil {
			return nil, err
		}
	}

	return index, nil
}

func (journal *Journal) applyIndex(index *journalIndex) {
	journal.events = index.events
	journal.roots = index.roots
	journal.effective = index.effective
	journal.natural = index.natural
	journal.order = index.order
}

func (journal *Journal) indexView() *journalIndex {
	return &journalIndex{
		events:    journal.events,
		roots:     journal.roots,
		effective: journal.effective,
		natural:   journal.natural,
		order:     journal.order,
	}
}

func prepareJournalDirectories(config Config) error {
	for _, directory := range []string{config.FilesRoot, config.JournalRoot} {
		if directory == "" {
			return fmt.Errorf("prepare journal: configured path is empty")
		}

		if err := os.MkdirAll(directory, 0o700); err != nil {
			return fmt.Errorf("prepare journal directory %q: %w", directory, err)
		}

		if err := os.Chmod(directory, 0o700); err != nil {
			return fmt.Errorf("set journal directory permissions %q: %w", directory, err)
		}
	}

	return nil
}

func listSegmentPaths(root string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return nil
		}

		if strings.HasSuffix(entry.Name(), ".jsonl") {
			paths = append(paths, current)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list journal segments: %w", err)
	}

	sort.Strings(paths)

	return paths, nil
}

func loadSegment(ctx context.Context, root, segmentPath string, index *journalIndex) (resultErr error) {
	file, err := os.Open(segmentPath)
	if err != nil {
		return fmt.Errorf("open journal segment %q: %w", segmentPath, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && resultErr == nil {
			resultErr = fmt.Errorf("close journal segment %q: %w", segmentPath, closeErr)
		}
	}()

	if err := validateSegmentTail(file, segmentPath); err != nil {
		return err
	}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), maxEventLineSize)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++

		if err := ctx.Err(); err != nil {
			return err
		}

		if err := loadSegmentActivity(root, segmentPath, lineNumber, scanner.Bytes(), index); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return corruptSegment(segmentPath, lineNumber+1, "read event: %v", err)
	}

	return nil
}

func loadSegmentActivity(root, segmentPath string, lineNumber int, line []byte, index *journalIndex) error {
	if len(line) == 0 {
		return corruptSegment(segmentPath, lineNumber, "blank line")
	}

	event, err := decodeCanonicalActivity(line)
	if err != nil {
		return corruptSegment(segmentPath, lineNumber, "%v", err)
	}

	if SegmentPath(root, event.RecordedAt) != segmentPath {
		return corruptSegment(segmentPath, lineNumber, "event belongs to %q", SegmentPath(root, event.RecordedAt))
	}

	if err := validateActivityAgainstIndex(event, index); err != nil {
		return corruptSegment(segmentPath, lineNumber, "%v", err)
	}

	addActivityToIndex(event, index)

	return nil
}

func decodeCanonicalActivity(line []byte) (Activity, error) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()

	var event Activity
	if err := decoder.Decode(&event); err != nil {
		return Activity{}, fmt.Errorf("decode event: %w", err)
	}

	if err := requireJSONEnd(decoder); err != nil {
		return Activity{}, fmt.Errorf("decode event: %w", err)
	}

	if err := event.Validate(); err != nil {
		return Activity{}, err
	}

	return event, nil
}

func canonicalActivityLine(event Activity) ([]byte, error) {
	line, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("encode event: %w", err)
	}

	return append(line, '\n'), nil
}

func validateActivityAgainstIndex(event Activity, index *journalIndex) error {
	if _, ok := index.events[event.ID]; ok {
		return fmt.Errorf("duplicate event ID %q", event.ID)
	}

	if event.Supersedes == "" {
		return validateNewActivityIdentity(event, index)
	}

	return validateSupersession(event, index)
}

func validateNewActivityIdentity(event Activity, index *journalIndex) error {
	key, ok := activityNaturalKey(event)
	if !ok {
		return nil
	}

	if _, exists := index.natural[key]; exists {
		return fmt.Errorf("duplicate natural key for %s", event.Type)
	}

	return nil
}

func validateSupersession(event Activity, index *journalIndex) error {
	target, ok := index.events[event.Supersedes]
	if !ok {
		return fmt.Errorf("correction target %q does not exist", event.Supersedes)
	}

	root := index.roots[target.ID]
	if index.effective[root] != target.ID {
		return fmt.Errorf("%w: %q", ErrCorrectionTargetStale, event.Supersedes)
	}

	if event.Type != target.Type {
		return fmt.Errorf("correction type does not match target %q", event.Supersedes)
	}

	if !sameNaturalIdentity(event, target) {
		return fmt.Errorf("correction identity does not match target %q", event.Supersedes)
	}

	return nil
}

func sameNaturalIdentity(left, right Activity) bool {
	leftKey, leftOK := activityNaturalKey(left)

	rightKey, rightOK := activityNaturalKey(right)
	if leftOK || rightOK {
		return leftOK && rightOK && leftKey == rightKey
	}

	if left.Type == ActivityTypeWorklog || left.Type == ActivityTypeNote {
		return true
	}

	return false
}

func addActivityToIndex(event Activity, index *journalIndex) {
	root := event.ID
	if event.Supersedes != "" {
		root = index.roots[event.Supersedes]
	}

	index.events[event.ID] = event
	index.roots[event.ID] = root
	index.effective[root] = event.ID
	index.order = append(index.order, event.ID)

	if key, ok := activityNaturalKey(event); ok {
		index.natural[key] = root
	}
}

func activityNaturalKey(event Activity) (naturalKey, bool) {
	if event.Properties == nil {
		return naturalKey{}, false
	}

	scopeIdentity := ""
	if event.Scope != nil {
		scopeIdentity = event.Scope.Identity
	}

	switch event.Type {
	case ActivityTypeCommit:
		return naturalKey{Type: event.Type, ScopeIdentity: scopeIdentity, Value: event.Properties.ObjectID}, true
	case ActivityTypePullRequest:
		value := fmt.Sprintf("%s\x00%s\x00%d\x00%s", event.Properties.Forge, event.Properties.Repository, event.Properties.Number, event.Properties.Action)

		return naturalKey{Type: event.Type, ScopeIdentity: scopeIdentity, Value: value}, true
	default:
		return naturalKey{}, false
	}
}

func appendEventLine(segmentPath string, line []byte) error {
	if err := os.MkdirAll(filepath.Dir(segmentPath), 0o700); err != nil {
		return fmt.Errorf("create segment directory: %w", err)
	}

	if err := os.Chmod(filepath.Dir(segmentPath), 0o700); err != nil {
		return fmt.Errorf("set segment directory permissions: %w", err)
	}

	file, err := os.OpenFile(segmentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open journal segment %q: %w", segmentPath, err)
	}

	if err := file.Chmod(0o600); err != nil {
		return closeJournalFile(file, fmt.Errorf("set journal segment permissions %q: %w", segmentPath, err))
	}

	if _, err := file.Write(line); err != nil {
		return closeJournalFile(file, fmt.Errorf("append journal segment %q: %w", segmentPath, err))
	}

	if err := file.Sync(); err != nil {
		return closeJournalFile(file, fmt.Errorf("sync journal segment %q: %w", segmentPath, err))
	}

	return closeJournalFile(file, nil)
}

func closeJournalFile(file *os.File, operationErr error) error {
	closeErr := file.Close()

	return errors.Join(operationErr, closeErr)
}

func corruptSegment(segmentPath string, line int, format string, arguments ...any) error {
	message := fmt.Sprintf(format, arguments...)

	return fmt.Errorf("%w: %s:%d: %s", ErrJournalCorrupt, segmentPath, line, message)
}

package lore

import (
	"errors"
	"fmt"
)

var (
	// ErrEntryRevisionTargetMissing indicates an unknown predecessor.
	ErrEntryRevisionTargetMissing = errors.New("entry revision target does not exist")
	// ErrEntryRevisionTargetStale indicates a predecessor that already has a
	// successor.
	ErrEntryRevisionTargetStale = errors.New("entry revision target is not effective")
)

type entryIndex struct {
	entries    map[EntryID]Entry
	roots      map[EntryID]EntryID
	effective  map[EntryID]EntryID
	successors map[EntryID]EntryID
	order      []EntryID
}

func newEntryIndex() *entryIndex {
	return &entryIndex{
		entries:    make(map[EntryID]Entry),
		roots:      make(map[EntryID]EntryID),
		effective:  make(map[EntryID]EntryID),
		successors: make(map[EntryID]EntryID),
		order:      make([]EntryID, 0),
	}
}

func validateEntryAgainstIndex(entry Entry, index *entryIndex) error {
	if _, exists := index.entries[entry.ID]; exists {
		return fmt.Errorf("append entry: duplicate entry ID %q", entry.ID)
	}

	if entry.Supersedes == "" {
		return nil
	}

	predecessor, exists := index.entries[entry.Supersedes]
	if !exists {
		return fmt.Errorf("%w: %q", ErrEntryRevisionTargetMissing, entry.Supersedes)
	}

	root := index.roots[predecessor.ID]
	if index.effective[root] != predecessor.ID {
		return fmt.Errorf("%w: %q", ErrEntryRevisionTargetStale, entry.Supersedes)
	}

	if entry.Kind != predecessor.Kind {
		return fmt.Errorf("append entry: kind does not match predecessor %q", entry.Supersedes)
	}

	if entry.RecordedAt.Before(predecessor.RecordedAt) {
		return fmt.Errorf("append entry: recorded_at precedes predecessor %q", entry.Supersedes)
	}

	return nil
}

func addEntryToIndex(entry Entry, index *entryIndex) {
	root := entry.ID
	if entry.Supersedes != "" {
		root = index.roots[entry.Supersedes]
		index.successors[entry.Supersedes] = entry.ID
	}

	index.entries[entry.ID] = entry
	index.roots[entry.ID] = root
	index.effective[root] = entry.ID
	index.order = append(index.order, entry.ID)
}

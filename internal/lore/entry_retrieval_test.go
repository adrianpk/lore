package lore

import (
	"testing"
	"time"
)

func TestSearchEntriesDefaultsToActiveNewestEntries(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	first := distinctKnowledgeEntry(t, 1)
	second := distinctKnowledgeEntry(t, 2)
	second.Kind = EntryKindFact
	second.Contexts = []Context{{Type: ContextTypeProject, Name: "other"}}
	third := distinctKnowledgeEntry(t, 3)
	third.Status = EntryStatusArchived

	appendKnowledgeEntry(t, journal, first)
	appendKnowledgeEntry(t, journal, second)
	appendKnowledgeEntry(t, journal, third)

	result, err := journal.SearchEntries(SearchEntriesParams{})
	if err != nil {
		t.Fatalf("SearchEntries() error = %v", err)
	}

	assertEntryIDs(t, result.Entries, []EntryID{second.ID, first.ID})
}

func TestSearchEntriesCombinesFilters(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	match := distinctKnowledgeEntry(t, 1)
	match.Title = "Recover Preview Cache"
	match.Body = "Inspect the service."
	match.Contexts = []Context{
		{Type: ContextTypeProject, Name: "lore"},
		{Type: ContextTypeHost, Name: "workstation"},
	}
	match.Labels = []string{"recovery", "service"}

	other := distinctKnowledgeEntry(t, 2)
	other.Title = "Deploy preview"

	appendKnowledgeEntry(t, journal, match)
	appendKnowledgeEntry(t, journal, other)

	result, err := journal.SearchEntries(SearchEntriesParams{
		Text:     "CACHE",
		Kinds:    []EntryKind{EntryKindProcedure, EntryKindWarning},
		Contexts: []Context{{Type: ContextTypeProject, Name: "Lore"}, {Type: ContextTypeHost, Name: "workstation"}},
		Labels:   []string{"recovery", "unrelated"},
	})
	if err != nil {
		t.Fatalf("SearchEntries() error = %v", err)
	}

	assertEntryIDs(t, result.Entries, []EntryID{match.ID})
}

func TestSearchEntriesBoundsAndOrdersResults(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	first := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 1))
	appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 2))

	result, err := journal.SearchEntries(SearchEntriesParams{Order: EntryOrderAscending, Limit: 1})
	if err != nil {
		t.Fatalf("SearchEntries() error = %v", err)
	}

	assertEntryIDs(t, result.Entries, []EntryID{first.ID})

	if !result.Truncated || result.ReturnedCount != 1 {
		t.Fatalf("search metadata = %#v", result)
	}

	if _, err := journal.SearchEntries(SearchEntriesParams{Limit: maximumEntrySearchLimit + 1}); err == nil {
		t.Fatal("expected maximum limit error")
	}
}

func TestShowEntryReturnsEffectiveRevisionAndHistory(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	root := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 1))
	revision := appendKnowledgeEntry(t, journal, entryRevision(t, root, 2, EntryStatusArchived))

	effective, err := journal.ShowEntry(ShowEntryParams{ID: root.ID})
	if err != nil {
		t.Fatalf("ShowEntry() error = %v", err)
	}

	if effective.Entry == nil || effective.Entry.ID != revision.ID {
		t.Fatalf("effective result = %#v", effective)
	}

	history, err := journal.ShowEntry(ShowEntryParams{ID: revision.ID, History: true})
	if err != nil {
		t.Fatalf("ShowEntry(history) error = %v", err)
	}

	assertEntryIDs(t, history.History, []EntryID{root.ID, revision.ID})
}

func TestSearchEntriesUsesRecordedInterval(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	first := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 1))
	second := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 2))
	from := first.RecordedAt.Add(time.Millisecond)
	to := second.RecordedAt.Add(time.Millisecond)

	result, err := journal.SearchEntries(SearchEntriesParams{RecordedFrom: &from, RecordedTo: &to})
	if err != nil {
		t.Fatalf("SearchEntries() error = %v", err)
	}

	assertEntryIDs(t, result.Entries, []EntryID{second.ID})
}

func assertEntryIDs(t *testing.T, entries []Entry, wanted []EntryID) {
	t.Helper()

	if len(entries) != len(wanted) {
		t.Fatalf("entry count = %d, want %d: %#v", len(entries), len(wanted), entries)
	}

	for index, entry := range entries {
		if entry.ID != wanted[index] {
			t.Fatalf("entry %d ID = %q, want %q", index, entry.ID, wanted[index])
		}
	}
}

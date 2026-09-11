package lore

import (
	"reflect"
	"testing"
	"time"
)

func TestReviewRequiresObjectiveSelector(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	if _, err := journal.Review(ReviewParams{}); err == nil {
		t.Fatal("expected objective selector error")
	}
}

func TestReviewSelectsWithoutMutation(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	old := distinctKnowledgeEntry(t, 1)
	old.Status = EntryStatusArchived
	old.Contexts = []Context{{Type: ContextTypeProject, Name: "lore"}}
	recent := distinctKnowledgeEntry(t, 2)
	recent.Status = EntryStatusArchived
	recent.Contexts = []Context{{Type: ContextTypeProject, Name: "other"}}

	appendKnowledgeEntry(t, journal, old)
	appendKnowledgeEntry(t, journal, recent)

	before := journal.EffectiveEntries()
	cutoff := old.RecordedAt.Add(time.Millisecond)

	result, err := journal.Review(ReviewParams{
		RecordedBefore: &cutoff,
		Statuses:       []EntryStatus{EntryStatusArchived},
		Contexts:       []Context{{Type: ContextTypeProject, Name: "Lore"}},
	})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}

	if len(result.Candidates) != 1 || result.Candidates[0].Entry.ID != old.ID {
		t.Fatalf("Review() = %#v", result)
	}

	if !reflect.DeepEqual(journal.EffectiveEntries(), before) || journal.Count() != 2 {
		t.Fatal("Review() mutated journal state")
	}
}

func TestReviewGroupsDuplicateCanonicalTitles(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	first := distinctKnowledgeEntry(t, 1)
	first.Title = "Recover Preview Service"
	second := distinctKnowledgeEntry(t, 2)
	second.Title = "recover   preview service"
	unique := distinctKnowledgeEntry(t, 3)
	unique.Title = "Inspect preview logs"

	appendKnowledgeEntry(t, journal, first)
	appendKnowledgeEntry(t, journal, second)
	appendKnowledgeEntry(t, journal, unique)

	result, err := journal.Review(ReviewParams{DuplicateTitles: true})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}

	assertEntryIDs(t, reviewEntries(result.Candidates), []EntryID{second.ID, first.ID})

	if len(result.DuplicateTitleGroups) != 1 {
		t.Fatalf("duplicate groups = %#v", result.DuplicateTitleGroups)
	}

	group := result.DuplicateTitleGroups[0]
	if group.Title != "recover preview service" || len(group.EntryIDs) != 2 {
		t.Fatalf("duplicate group = %#v", group)
	}
}

func TestReviewBoundsCandidates(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	for sequence := 1; sequence <= 3; sequence++ {
		entry := distinctKnowledgeEntry(t, sequence)
		entry.Status = EntryStatusInvalidated
		appendKnowledgeEntry(t, journal, entry)
	}

	result, err := journal.Review(ReviewParams{
		Statuses: []EntryStatus{EntryStatusInvalidated},
		Limit:    2,
	})
	if err != nil {
		t.Fatalf("Review() error = %v", err)
	}

	if !result.Truncated || result.ReturnedCount != 2 || len(result.Candidates) != 2 {
		t.Fatalf("bounded result = %#v", result)
	}
}

func reviewEntries(candidates []ReviewCandidate) []Entry {
	entries := make([]Entry, len(candidates))
	for index, candidate := range candidates {
		entries[index] = candidate.Entry
	}

	return entries
}

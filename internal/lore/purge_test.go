package lore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPurgeDryRunSelectsCompleteChainsWithoutMutation(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	root := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 1))
	revision := appendKnowledgeEntry(t, journal, entryRevision(t, root, 2, EntryStatusArchived))
	keep := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 3))
	before := journal.EffectiveEntries()

	result, err := journal.Purge(context.Background(), PurgeParams{
		IDs:    []EntryID{revision.ID},
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("Purge(dry-run) error = %v", err)
	}

	if result.ChainCount != 1 || result.RevisionCount != 2 || result.ConfirmationToken == "" {
		t.Fatalf("dry-run result = %#v", result)
	}

	assertEntryIDSlice(t, result.Chains[0].EntryIDs, []EntryID{root.ID, revision.ID})

	if !reflect.DeepEqual(journal.EffectiveEntries(), before) || journal.Count() != 3 {
		t.Fatal("dry-run mutated journal")
	}

	if _, exists := journal.Entry(keep.ID); !exists {
		t.Fatal("dry-run removed retained entry")
	}
}

func TestPurgeRejectsMismatchedConfirmation(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	entry := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 1))

	_, err := journal.Purge(context.Background(), PurgeParams{
		IDs:     []EntryID{entry.ID},
		Confirm: "not-a-valid-token",
	})
	if !errors.Is(err, ErrPurgeConfirmation) {
		t.Fatalf("Purge(confirm) error = %v, want ErrPurgeConfirmation", err)
	}

	if journal.Count() != 1 {
		t.Fatal("mismatched confirmation mutated journal")
	}
}

func TestPurgeConfirmedRemovesCompleteChains(t *testing.T) {
	t.Parallel()

	config := entryJournalConfig(t)
	journal := loadEntryJournal(t, config)
	root := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 1))
	revision := appendKnowledgeEntry(t, journal, entryRevision(t, root, 2, EntryStatusInvalidated))
	keep := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 3))
	params := PurgeParams{Statuses: []EntryStatus{EntryStatusInvalidated}, DryRun: true}

	dryRun, err := journal.Purge(context.Background(), params)
	if err != nil {
		t.Fatalf("Purge(dry-run) error = %v", err)
	}

	params.DryRun = false
	params.Confirm = dryRun.ConfirmationToken

	result, err := journal.Purge(context.Background(), params)
	if err != nil {
		t.Fatalf("Purge(confirm) error = %v", err)
	}

	if !result.Purged || journal.Count() != 1 {
		t.Fatalf("confirmed result = %#v, count = %d", result, journal.Count())
	}

	if _, exists := journal.Entry(root.ID); exists {
		t.Fatal("purged root remains")
	}

	if _, exists := journal.Entry(revision.ID); exists {
		t.Fatal("purged revision remains")
	}

	reloaded := loadEntryJournal(t, config)
	if reloaded.Count() != 1 {
		t.Fatalf("reloaded count = %d", reloaded.Count())
	}

	if _, exists := reloaded.Entry(keep.ID); !exists {
		t.Fatal("retained entry missing after reload")
	}
}

func TestPurgeStoreChangeInvalidatesToken(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	target := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 1))

	dryRun, err := journal.Purge(context.Background(), PurgeParams{IDs: []EntryID{target.ID}, DryRun: true})
	if err != nil {
		t.Fatalf("Purge(dry-run) error = %v", err)
	}

	appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 2))

	_, err = journal.Purge(context.Background(), PurgeParams{
		IDs:     []EntryID{target.ID},
		Confirm: dryRun.ConfirmationToken,
	})
	if !errors.Is(err, ErrPurgeConfirmation) {
		t.Fatalf("Purge(confirm) error = %v, want ErrPurgeConfirmation", err)
	}
}

func TestPurgeRecoveryCompletesDirectorySwap(t *testing.T) {
	t.Parallel()

	config := entryJournalConfig(t)
	journal := loadEntryJournal(t, config)
	appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 1))

	staging, backup := purgeWorkPaths(config)

	if err := os.Mkdir(staging, 0o700); err != nil {
		t.Fatalf("create staging directory: %v", err)
	}

	if err := os.Rename(config.EntriesRoot, backup); err != nil {
		t.Fatalf("stage live directory: %v", err)
	}

	reloaded := loadEntryJournal(t, config)
	if reloaded.Count() != 0 {
		t.Fatalf("recovered count = %d, want 0", reloaded.Count())
	}

	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup stat error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(config.EntriesRoot)); err != nil {
		t.Fatalf("live entry root stat error = %v", err)
	}
}

func TestPurgeRequiresExplicitModeAndSelector(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	tests := []PurgeParams{
		{},
		{DryRun: true},
		{All: true},
		{All: true, IDs: []EntryID{"01M26K00000000000000000000"}, DryRun: true},
	}

	for _, params := range tests {
		if _, err := journal.Purge(context.Background(), params); !errors.Is(err, ErrPurgeSelector) {
			t.Errorf("Purge(%#v) error = %v, want ErrPurgeSelector", params, err)
		}
	}
}

func assertEntryIDSlice(t *testing.T, got, want []EntryID) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entry IDs = %#v, want %#v", got, want)
	}
}

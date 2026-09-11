package lore

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestRecordEntryAppendsCanonicalRoot(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	workflows := deterministicEntryWorkflows(journal, 10)

	result, err := workflows.Record(context.Background(), RecordEntryParams{
		Kind:   EntryKindWarning,
		Title:  "  Inspect Before Restarting  ",
		Body:   "  Confirm the failure first.  ",
		Labels: []string{"Service Recovery"},
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	if result.Entry.Status != EntryStatusActive || result.Entry.Supersedes != "" {
		t.Fatalf("recorded entry = %#v", result.Entry)
	}

	if result.Entry.Title != "Inspect Before Restarting" || result.Entry.Labels[0] != "service-recovery" {
		t.Fatalf("canonical entry = %#v", result.Entry)
	}

	if journal.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", journal.Count())
	}
}

func TestReviseEntryInheritsAndReplacesCompleteFields(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	root := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 1))
	workflows := deterministicEntryWorkflows(journal, 20)

	title := "Updated recovery procedure"
	labels := []string{"updated"}

	result, err := workflows.Revise(context.Background(), ReviseEntryParams{
		Target:          root.ID,
		Reason:          "Clarified the recovered workflow.",
		Title:           &title,
		Labels:          &labels,
		ClearVerifiedAt: true,
	})
	if err != nil {
		t.Fatalf("Revise() error = %v", err)
	}

	if result.Entry.Title != title || result.Entry.Body != root.Body {
		t.Fatalf("revised content = %#v", result.Entry)
	}

	if result.Entry.VerifiedAt != nil || result.Entry.Labels[0] != "updated" {
		t.Fatalf("revised metadata = %#v", result.Entry)
	}

	if result.Entry.Supersedes != root.ID || result.Entry.RevisionReason == "" {
		t.Fatalf("revision linkage = %#v", result.Entry)
	}
}

func TestReviseEntryRejectsStaleTarget(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	root := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 1))
	workflows := deterministicEntryWorkflows(journal, 20)

	if _, err := workflows.ChangeStatus(
		context.Background(),
		LifecycleEntryParams{Target: root.ID, Reason: "No longer current."},
		EntryStatusArchived,
	); err != nil {
		t.Fatalf("ChangeStatus() error = %v", err)
	}

	_, err := workflows.Revise(context.Background(), ReviseEntryParams{Target: root.ID, Reason: "Stale edit."})
	if !errors.Is(err, ErrEntryTargetStale) {
		t.Fatalf("Revise() error = %v, want ErrEntryTargetStale", err)
	}
}

func TestEntryLifecycleAppendsStatusRevisions(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	root := appendKnowledgeEntry(t, journal, distinctKnowledgeEntry(t, 1))
	workflows := deterministicEntryWorkflows(journal, 20)

	archived, err := workflows.ChangeStatus(
		context.Background(),
		LifecycleEntryParams{Target: root.ID, Reason: "Retired."},
		EntryStatusArchived,
	)
	if err != nil {
		t.Fatalf("archive ChangeStatus() error = %v", err)
	}

	workflows = deterministicEntryWorkflows(journal, 30)

	restored, err := workflows.ChangeStatus(
		context.Background(),
		LifecycleEntryParams{Target: archived.Entry.ID, Reason: "Relevant again."},
		EntryStatusActive,
	)
	if err != nil {
		t.Fatalf("restore ChangeStatus() error = %v", err)
	}

	if restored.Entry.Status != EntryStatusActive {
		t.Fatalf("restored status = %q", restored.Entry.Status)
	}

	history, exists := journal.History(root.ID)
	if !exists || len(history) != 3 {
		t.Fatalf("History() = %#v, %t", history, exists)
	}
}

func deterministicEntryWorkflows(journal *EntryJournal, marker byte) *EntryWorkflows {
	workflows := NewEntryWorkflows(journal)
	workflows.now = func() time.Time {
		return time.Date(2026, 9, 11, 10, 0, int(marker), 0, time.UTC)
	}
	workflows.entropy = bytes.NewReader(bytes.Repeat([]byte{marker}, 10))

	return workflows
}

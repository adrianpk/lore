package lore

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCorrectorAppendsSelfContainedReplacement(t *testing.T) {
	t.Parallel()

	journal := loadedJournalWithEvent(t, validMilestoneEvent())
	target := validMilestoneEvent()
	corrector := deterministicCorrector(journal)
	summary := "Replacement significance summary"

	result, err := corrector.Correct(context.Background(), CorrectParams{
		TargetID:           target.ID,
		Reason:             "Clarify the final behavior",
		ReplacementSummary: &summary,
	})
	if err != nil {
		t.Fatalf("Correct() error = %v", err)
	}

	event := result.Event
	assertReplacementSnapshot(t, event, target, summary)

	effective, ok := journal.Effective(target.ID)
	if !ok || effective.ID != event.ID {
		t.Fatalf("Effective() = %#v, %v", effective, ok)
	}
}

func assertReplacementSnapshot(t *testing.T, event, target Activity, summary string) {
	t.Helper()

	if event.Type != target.Type || event.Supersedes != target.ID || event.Summary != summary {
		t.Fatalf("Correct() event = %#v", event)
	}

	if event.Scope == target.Scope || *event.Scope != *target.Scope {
		t.Fatalf("correction scope = %#v", event.Scope)
	}

	if event.Properties == target.Properties || !reflect.DeepEqual(*event.Properties, *target.Properties) {
		t.Fatalf("correction properties = %#v", event.Properties)
	}

	if event.Provenance == target.Provenance || *event.Provenance != *target.Provenance {
		t.Fatalf("correction provenance = %#v", event.Provenance)
	}
}

func TestCorrectorPreservesSummaryWhenReplacementIsAbsent(t *testing.T) {
	t.Parallel()

	target := validMilestoneEvent()
	journal := loadedJournalWithEvent(t, target)

	result, err := deterministicCorrector(journal).Correct(context.Background(), CorrectParams{
		TargetID: target.ID,
		Reason:   "Preserve the effective summary",
	})
	if err != nil {
		t.Fatalf("Correct() error = %v", err)
	}

	if result.Event.Summary != target.Summary {
		t.Fatalf("correction summary = %q, want %q", result.Event.Summary, target.Summary)
	}
}

func TestCorrectorChangesWorklogOccurrenceAndDuration(t *testing.T) {
	t.Parallel()

	target := validWorklogActivity()
	journal := loadedJournalWithEvent(t, target)
	occurredAt := target.OccurredAt.Add(-2 * time.Hour)
	duration := 90

	result, err := deterministicCorrector(journal).Correct(context.Background(), CorrectParams{
		TargetID:                   target.ID,
		Reason:                     "Correct the timesheet facts",
		ReplacementOccurredAt:      &occurredAt,
		ReplacementDurationMinutes: &duration,
	})
	if err != nil {
		t.Fatalf("Correct() error = %v", err)
	}

	if !result.Event.OccurredAt.Equal(occurredAt) || result.Event.Properties.DurationMinutes != duration {
		t.Fatalf("corrected worklog = %#v", result.Event)
	}
}

func TestCorrectorRejectsDurationForNonWorklog(t *testing.T) {
	t.Parallel()

	target := validNoteActivity()
	journal := loadedJournalWithEvent(t, target)
	duration := 30

	_, err := deterministicCorrector(journal).Correct(context.Background(), CorrectParams{
		TargetID:                   target.ID,
		Reason:                     "Invalid duration correction",
		ReplacementDurationMinutes: &duration,
	})
	if !errors.Is(err, ErrEventInvalid) {
		t.Fatalf("Correct() error = %v, want ErrEventInvalid", err)
	}
}

func TestCorrectorRejectsMissingStaleAndInvalidTargets(t *testing.T) {
	t.Parallel()

	target := validMilestoneEvent()
	journal := loadedJournalWithEvent(t, target)

	first, err := deterministicCorrector(journal).Correct(context.Background(), CorrectParams{
		TargetID: target.ID,
		Reason:   "Create an effective replacement",
	})
	if err != nil {
		t.Fatalf("create correction: %v", err)
	}

	tests := []struct {
		name   string
		params CorrectParams
		want   error
	}{
		{name: "missing", params: CorrectParams{TargetID: "01K1Z8V7BMMN6W7Y6R8QJHA9ZZ", Reason: "Missing target"}, want: ErrCorrectionTargetNotFound},
		{name: "stale", params: CorrectParams{TargetID: target.ID, Reason: "Stale target"}, want: ErrCorrectionTargetStale},
		{name: "invalid reason", params: CorrectParams{TargetID: first.Event.ID}, want: ErrEventInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := deterministicCorrector(journal).Correct(context.Background(), test.params); !errors.Is(err, test.want) {
				t.Fatalf("Correct() error = %v, want %v", err, test.want)
			}
		})
	}
}

func loadedJournalWithEvent(t *testing.T, event Event) *Journal {
	t.Helper()

	journal := NewJournal(testJournalConfig(t))
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("load journal: %v", err)
	}

	appendJournalEvent(t, journal, event)

	return journal
}

func deterministicCorrector(journal *Journal) *Corrector {
	corrector := NewCorrector(journal)
	corrector.now = func() time.Time {
		return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	}
	corrector.entropy = strings.NewReader(strings.Repeat("\x01", 10))

	return corrector
}

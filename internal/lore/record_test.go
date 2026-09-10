package lore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRecorderPublishesIdempotentCommit(t *testing.T) {
	t.Parallel()

	recorder, journal := testRecorder(t)
	params := validRecordParams()

	first, err := recorder.Record(context.Background(), params)
	if err != nil {
		t.Fatalf("first Record() error = %v", err)
	}

	second, err := recorder.Record(context.Background(), params)
	if err != nil {
		t.Fatalf("second Record() error = %v", err)
	}

	assertIdempotentRecord(t, journal, params, first, second)
}

func assertIdempotentRecord(t *testing.T, journal *Journal, params RecordParams, first, second RecordResult) {
	t.Helper()

	wantFirst := RecordResult{
		ID:         first.ID,
		Recorded:   true,
		Type:       ActivityTypeCommit,
		ScopeName:  params.Scope.Name,
		Identifier: abbreviatedCommit(params.Properties.ObjectID),
	}
	if first != wantFirst {
		t.Fatalf("first record result = %#v, want %#v", first, wantFirst)
	}

	wantSecond := wantFirst

	wantSecond.Recorded = false
	if second != wantSecond {
		t.Fatalf("second record result = %#v, want %#v", second, wantSecond)
	}

	if journal.Count() != 1 {
		t.Fatalf("journal count = %d, want 1", journal.Count())
	}

	event, ok := journal.Event(first.ID)
	if !ok {
		t.Fatal("recorded event is missing")
	}

	if event.Summary != params.Summary || !event.RecordedAt.Equal(time.Date(2026, 8, 5, 18, 0, 0, 0, time.UTC)) {
		t.Fatalf("recorded event = %#v", event)
	}
}

func TestRecorderPreservesOptionalProvenance(t *testing.T) {
	t.Parallel()

	recorder, journal := testRecorder(t)
	params := validRecordParams()
	params.Provenance = &ActivityProvenance{Type: ProvenanceTypeAgent, Harness: "codex", HarnessVersion: "1.2.3", Model: "openai/example-model"}

	result, err := recorder.Record(context.Background(), params)
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}

	event, ok := journal.Event(result.ID)
	if !ok || event.Provenance == nil || *event.Provenance != *params.Provenance {
		t.Fatalf("recorded provenance = %#v", event.Provenance)
	}
}

func TestRecorderRejectsInvalidActivity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change func(*RecordParams)
	}{
		{name: "missing type", change: func(params *RecordParams) { params.Type = "" }},
		{name: "multiline summary", change: func(params *RecordParams) { params.Summary = "important\nsecret" }},
		{name: "invalid scope", change: func(params *RecordParams) { params.Scope.Identity = "" }},
		{name: "invalid provenance", change: func(params *RecordParams) {
			params.Provenance = &ActivityProvenance{Type: ProvenanceTypeAgent, Harness: "codex\nagent"}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder, journal := testRecorder(t)
			params := validRecordParams()
			test.change(&params)

			if _, err := recorder.Record(context.Background(), params); !errors.Is(err, ErrEventInvalid) {
				t.Fatalf("Record() error = %v, want ErrEventInvalid", err)
			}

			if journal.Count() != 0 {
				t.Fatalf("journal count = %d, want 0", journal.Count())
			}
		})
	}
}

func TestRecorderReportsEntropyFailureAndRequiresLoadedJournal(t *testing.T) {
	t.Parallel()

	recorder, _ := testRecorder(t)

	recorder.entropy = failingReader{}
	if _, err := recorder.Record(context.Background(), validRecordParams()); err == nil {
		t.Fatal("expected entropy error")
	}

	unloaded := NewRecorder(NewJournal(testJournalConfig(t)))
	if _, err := unloaded.Record(context.Background(), validRecordParams()); !errors.Is(err, ErrJournalNotLoaded) {
		t.Fatalf("Record() error = %v, want ErrJournalNotLoaded", err)
	}
}

func testRecorder(t *testing.T) (*Recorder, *Journal) {
	t.Helper()

	journal := NewJournal(testJournalConfig(t))
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("load journal: %v", err)
	}

	recorder := NewRecorder(journal)
	recorder.now = func() time.Time {
		return time.Date(2026, 8, 5, 20, 0, 0, 0, time.FixedZone("test", 2*60*60))
	}
	recorder.entropy = strings.NewReader(strings.Repeat("\x00", 20))

	return recorder, journal
}

func validRecordParams() RecordParams {
	event := validMilestoneEvent()

	return RecordParams{
		Type:       event.Type,
		OccurredAt: event.OccurredAt,
		Summary:    event.Summary,
		Scope:      event.Scope,
		Properties: event.Properties,
		References: event.References,
		Provenance: event.Provenance,
	}
}

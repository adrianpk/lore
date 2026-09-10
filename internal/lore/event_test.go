package lore

import (
	"testing"
	"time"
)

func TestEventAliasUsesCanonicalActivityValidation(t *testing.T) {
	t.Parallel()

	event := validMilestoneEvent()
	if err := event.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func validMilestoneEvent() Event {
	authoredAt := time.Date(2026, 8, 5, 14, 18, 3, 0, time.UTC)
	recordedAt := time.Date(2026, 8, 5, 14, 22, 31, 418000000, time.UTC)

	return Event{
		ID:         testEventID(recordedAt, 2),
		Type:       ActivityTypeCommit,
		OccurredAt: time.Date(2026, 8, 5, 14, 20, 47, 0, time.UTC),
		RecordedAt: recordedAt,
		Summary:    "Reusable protocol behavior across agent-enabled projects",
		Scope: &ActivityScope{
			Type:     ScopeTypeRepository,
			Name:     "example",
			Identity: "ssh://git@example.test/team/example.git",
		},
		Properties: &ActivityProperties{
			ObjectID:   "0123456789abcdef0123456789abcdef01234567",
			Subject:    "feat: add reusable agent protocol",
			AuthoredAt: &authoredAt,
		},
		Provenance: &ActivityProvenance{
			Type:           ProvenanceTypeAgent,
			Harness:        "pi",
			HarnessVersion: "0.42.0",
			Model:          "openai/gpt-5.6",
		},
	}
}

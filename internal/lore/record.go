package lore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// ErrEventInvalid identifies input that cannot form a valid activity.
var ErrEventInvalid = errors.New("event is invalid")

// RecordParams contains one typed activity supplied to the service.
type RecordParams struct {
	Type       ActivityType        `json:"type"`
	OccurredAt time.Time           `json:"occurred_at,omitempty"`
	Summary    string              `json:"summary"`
	Scope      *ActivityScope      `json:"scope,omitempty"`
	Properties *ActivityProperties `json:"properties,omitempty"`
	References []ActivityReference `json:"references,omitempty"`
	Provenance *ActivityProvenance `json:"provenance,omitempty"`
}

// RecordResult describes one newly recorded or existing activity.
type RecordResult struct {
	ID         EventID      `json:"id"`
	Recorded   bool         `json:"recorded"`
	Type       ActivityType `json:"type"`
	ScopeName  string       `json:"scope_name,omitempty"`
	Identifier string       `json:"identifier,omitempty"`
}

// Recorder creates service-owned event identity and publication timestamps.
type Recorder struct {
	journal *Journal
	now     func() time.Time
	entropy io.Reader
}

// NewRecorder constructs a recorder without performing IO.
func NewRecorder(journal *Journal) *Recorder {
	return &Recorder{journal: journal, now: time.Now}
}

// Record validates and appends one typed activity.
func (recorder *Recorder) Record(ctx context.Context, params RecordParams) (RecordResult, error) {
	recordedAt := recorder.now().UTC()

	id, err := NewEventID(recordedAt, recorder.entropy)
	if err != nil {
		return RecordResult{}, err
	}

	occurredAt := params.OccurredAt.UTC()
	if params.OccurredAt.IsZero() {
		occurredAt = recordedAt
	}

	event := Activity{
		ID:         id,
		Type:       params.Type,
		OccurredAt: occurredAt,
		RecordedAt: recordedAt,
		Summary:    params.Summary,
		Scope:      params.Scope,
		Properties: params.Properties,
		References: params.References,
		Provenance: params.Provenance,
	}
	if err := event.Validate(); err != nil {
		return RecordResult{}, fmt.Errorf("%w: %w", ErrEventInvalid, err)
	}

	result, err := recorder.journal.Append(ctx, event)
	if err != nil {
		return RecordResult{}, err
	}

	return recordResult(result), nil
}

func recordResult(result AppendResult) RecordResult {
	event := result.Event

	return RecordResult{
		ID:         event.ID,
		Recorded:   result.Recorded,
		Type:       event.Type,
		ScopeName:  activityScopeName(event),
		Identifier: activityIdentifier(event),
	}
}

func activityScopeName(event Activity) string {
	if event.Scope == nil {
		return ""
	}

	return event.Scope.Name
}

func activityIdentifier(event Activity) string {
	if event.Properties == nil {
		return ""
	}

	switch event.Type {
	case ActivityTypeCommit:
		return abbreviatedCommit(event.Properties.ObjectID)
	case ActivityTypePullRequest:
		return fmt.Sprintf("%s#%d:%s", event.Properties.Repository, event.Properties.Number, event.Properties.Action)
	default:
		return ""
	}
}

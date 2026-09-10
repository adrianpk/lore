package lore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// ErrCorrectionTargetNotFound identifies a missing correction target.
var ErrCorrectionTargetNotFound = errors.New("correction target was not found")

// ErrCorrectionTargetStale identifies a target that is no longer effective.
var ErrCorrectionTargetStale = errors.New("correction target is not effective")

// CorrectParams contains append-only correction input.
type CorrectParams struct {
	TargetID                   EventID    `json:"target_id"`
	Reason                     string     `json:"reason"`
	ReplacementOccurredAt      *time.Time `json:"replacement_occurred_at,omitempty"`
	ReplacementSummary         *string    `json:"replacement_summary,omitempty"`
	ReplacementDurationMinutes *int       `json:"replacement_duration_minutes,omitempty"`
}

// CorrectResult describes a newly appended correction.
type CorrectResult struct {
	Event Activity `json:"event"`
}

// Corrector creates service-owned correction identity and timestamps.
type Corrector struct {
	journal *Journal
	now     func() time.Time
	entropy io.Reader
}

// NewCorrector constructs a corrector without performing IO.
func NewCorrector(journal *Journal) *Corrector {
	return &Corrector{journal: journal, now: time.Now}
}

// Correct appends a complete replacement for one effective activity.
func (corrector *Corrector) Correct(ctx context.Context, params CorrectParams) (CorrectResult, error) {
	if !corrector.journal.Loaded() {
		return CorrectResult{}, ErrJournalNotLoaded
	}

	target, ok := corrector.journal.Event(params.TargetID)
	if !ok {
		return CorrectResult{}, ErrCorrectionTargetNotFound
	}

	effective, ok := corrector.journal.Effective(target.ID)
	if !ok || effective.ID != target.ID {
		return CorrectResult{}, ErrCorrectionTargetStale
	}

	event, err := corrector.replacement(target, params)
	if err != nil {
		return CorrectResult{}, err
	}

	result, err := corrector.journal.Append(ctx, event)
	if err != nil {
		return CorrectResult{}, err
	}

	return CorrectResult{Event: result.Event}, nil
}

func (corrector *Corrector) replacement(target Activity, params CorrectParams) (Activity, error) {
	recordedAt := corrector.now().UTC()

	id, err := NewEventID(recordedAt, corrector.entropy)
	if err != nil {
		return Activity{}, err
	}

	event := cloneActivity(target)
	event.ID = id
	event.RecordedAt = recordedAt
	event.Supersedes = target.ID
	event.CorrectionReason = params.Reason

	if params.ReplacementOccurredAt != nil {
		event.OccurredAt = params.ReplacementOccurredAt.UTC()
	}

	if params.ReplacementSummary != nil {
		event.Summary = *params.ReplacementSummary
	}

	if params.ReplacementDurationMinutes != nil {
		if event.Type != ActivityTypeWorklog || event.Properties == nil {
			return Activity{}, fmt.Errorf("%w: only worklogs have duration", ErrEventInvalid)
		}

		event.Properties.DurationMinutes = *params.ReplacementDurationMinutes
	}

	if err := event.Validate(); err != nil {
		return Activity{}, fmt.Errorf("%w: %w", ErrEventInvalid, err)
	}

	return event, nil
}

func cloneActivity(activity Activity) Activity {
	if activity.Scope != nil {
		scope := *activity.Scope
		activity.Scope = &scope
	}

	if activity.Properties != nil {
		properties := *activity.Properties
		if properties.AuthoredAt != nil {
			authoredAt := *properties.AuthoredAt
			properties.AuthoredAt = &authoredAt
		}

		activity.Properties = &properties
	}

	if activity.Provenance != nil {
		provenance := *activity.Provenance
		activity.Provenance = &provenance
	}

	activity.References = append([]ActivityReference(nil), activity.References...)

	return activity
}

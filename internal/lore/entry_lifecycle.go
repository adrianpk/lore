package lore

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrEntryNotFound indicates that a requested knowledge entry does not exist.
	ErrEntryNotFound = errors.New("entry does not exist")
	// ErrEntryTargetStale indicates that a mutation did not target the effective revision.
	ErrEntryTargetStale = errors.New("entry target is not effective")
)

// ReviseEntryParams contains the target and explicitly replaced entry fields.
type ReviseEntryParams struct {
	Target          EntryID           `json:"target"`
	Reason          string            `json:"reason"`
	Title           *string           `json:"title,omitempty"`
	Body            *string           `json:"body,omitempty"`
	VerifiedAt      *time.Time        `json:"verified_at,omitempty"`
	ClearVerifiedAt bool              `json:"clear_verified_at,omitempty"`
	Contexts        *[]Context        `json:"contexts,omitempty"`
	Labels          *[]string         `json:"labels,omitempty"`
	References      *[]EntryReference `json:"references,omitempty"`
	Provenance      *Provenance       `json:"provenance,omitempty"`
	ClearProvenance bool              `json:"clear_provenance,omitempty"`
}

// LifecycleEntryParams contains one explicit entry state transition.
type LifecycleEntryParams struct {
	Target EntryID `json:"target"`
	Reason string  `json:"reason"`
}

// Revise appends a complete successor, inheriting fields omitted by the caller.
func (workflows *EntryWorkflows) Revise(ctx context.Context, params ReviseEntryParams) (EntryMutationResult, error) {
	previous, err := workflows.requireEffective(params.Target)
	if err != nil {
		return EntryMutationResult{}, err
	}

	replacement := applyEntryReplacements(previous, params)

	return workflows.appendRevision(ctx, previous, replacement, params.Reason)
}

// ChangeStatus appends a complete lifecycle successor.
func (workflows *EntryWorkflows) ChangeStatus(ctx context.Context, params LifecycleEntryParams, status EntryStatus) (EntryMutationResult, error) {
	if !status.Valid() {
		return EntryMutationResult{}, fmt.Errorf("change entry status: unsupported status %q", status)
	}

	previous, err := workflows.requireEffective(params.Target)
	if err != nil {
		return EntryMutationResult{}, err
	}

	replacement := cloneEntry(previous)
	replacement.Status = status

	return workflows.appendRevision(ctx, previous, replacement, params.Reason)
}

func (workflows *EntryWorkflows) appendRevision(ctx context.Context, previous, replacement Entry, reason string) (EntryMutationResult, error) {
	recordedAt := workflows.now().UTC()

	id, err := NewEntryID(recordedAt, workflows.entropy)
	if err != nil {
		return EntryMutationResult{}, err
	}

	entry, err := NewEntryRevision(previous, replacement, id, recordedAt, reason)
	if err != nil {
		return EntryMutationResult{}, err
	}

	return workflows.append(ctx, entry)
}

func (workflows *EntryWorkflows) requireEffective(target EntryID) (Entry, error) {
	if err := target.Validate(); err != nil {
		return Entry{}, err
	}

	effective, exists := workflows.journal.Effective(target)
	if !exists {
		return Entry{}, fmt.Errorf("%w: %q", ErrEntryNotFound, target)
	}

	if effective.ID != target {
		return Entry{}, fmt.Errorf("%w: %q", ErrEntryTargetStale, target)
	}

	return effective, nil
}

func applyEntryReplacements(previous Entry, params ReviseEntryParams) Entry {
	replacement := cloneEntry(previous)

	if params.Title != nil {
		replacement.Title = *params.Title
	}

	if params.Body != nil {
		replacement.Body = *params.Body
	}

	if params.ClearVerifiedAt {
		replacement.VerifiedAt = nil
	} else if params.VerifiedAt != nil {
		replacement.VerifiedAt = cloneTime(params.VerifiedAt)
	}

	if params.Contexts != nil {
		replacement.Contexts = append([]Context(nil), (*params.Contexts)...)
	}

	if params.Labels != nil {
		replacement.Labels = append([]string(nil), (*params.Labels)...)
	}

	if params.References != nil {
		replacement.References = append([]EntryReference(nil), (*params.References)...)
	}

	if params.ClearProvenance {
		replacement.Provenance = nil
	} else if params.Provenance != nil {
		provenance := *params.Provenance
		replacement.Provenance = &provenance
	}

	return replacement
}

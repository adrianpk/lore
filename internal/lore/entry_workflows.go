package lore

import (
	"context"
	"crypto/rand"
	"io"
	"time"
)

// RecordEntryParams contains caller-supplied content for a new root entry.
type RecordEntryParams struct {
	Kind       EntryKind        `json:"kind"`
	Title      string           `json:"title"`
	Body       string           `json:"body"`
	VerifiedAt *time.Time       `json:"verified_at,omitempty"`
	Contexts   []Context        `json:"contexts,omitempty"`
	Labels     []string         `json:"labels,omitempty"`
	References []EntryReference `json:"references,omitempty"`
	Provenance *Provenance      `json:"provenance,omitempty"`
}

// EntryMutationResult reports the complete revision appended by a mutation.
type EntryMutationResult struct {
	Entry Entry `json:"entry"`
}

// EntryWorkflows owns knowledge capture and complete-revision mutations.
type EntryWorkflows struct {
	journal *EntryJournal
	now     func() time.Time
	entropy io.Reader
}

// NewEntryWorkflows constructs entry workflows around one loaded journal.
func NewEntryWorkflows(journal *EntryJournal) *EntryWorkflows {
	return &EntryWorkflows{journal: journal, now: time.Now, entropy: rand.Reader}
}

// Record validates and appends one active root entry.
func (workflows *EntryWorkflows) Record(ctx context.Context, params RecordEntryParams) (EntryMutationResult, error) {
	recordedAt := workflows.now().UTC()

	id, err := NewEntryID(recordedAt, workflows.entropy)
	if err != nil {
		return EntryMutationResult{}, err
	}

	entry := Entry{
		ID:         id,
		Kind:       params.Kind,
		Status:     EntryStatusActive,
		Title:      params.Title,
		Body:       params.Body,
		RecordedAt: recordedAt,
		VerifiedAt: cloneTime(params.VerifiedAt),
		Contexts:   params.Contexts,
		Labels:     params.Labels,
		References: params.References,
		Provenance: params.Provenance,
	}

	return workflows.append(ctx, entry)
}

func (workflows *EntryWorkflows) append(ctx context.Context, entry Entry) (EntryMutationResult, error) {
	result, err := workflows.journal.Append(ctx, entry)
	if err != nil {
		return EntryMutationResult{}, err
	}

	return EntryMutationResult(result), nil
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}

package lore

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	defaultEntrySearchLimit = 20
	maximumEntrySearchLimit = 200
)

// EntryOrder selects deterministic recording-time ordering.
type EntryOrder string

const (
	// EntryOrderAscending returns the oldest matching entries first.
	EntryOrderAscending EntryOrder = "ascending"
	// EntryOrderDescending returns the newest matching entries first.
	EntryOrderDescending EntryOrder = "descending"
)

// SearchEntriesParams contains bounded effective-entry filters.
type SearchEntriesParams struct {
	Text         string        `json:"text,omitempty"`
	Kinds        []EntryKind   `json:"kinds,omitempty"`
	Statuses     []EntryStatus `json:"statuses,omitempty"`
	Contexts     []Context     `json:"contexts,omitempty"`
	Labels       []string      `json:"labels,omitempty"`
	RecordedFrom *time.Time    `json:"recorded_from,omitempty"`
	RecordedTo   *time.Time    `json:"recorded_to,omitempty"`
	Order        EntryOrder    `json:"order,omitempty"`
	Limit        int           `json:"limit,omitempty"`
}

// SearchEntriesResult contains complete effective entries and truncation state.
type SearchEntriesResult struct {
	Entries       []Entry `json:"entries"`
	ReturnedCount int     `json:"returned_count"`
	Truncated     bool    `json:"truncated"`
}

// ShowEntryParams selects one effective entry or its complete history.
type ShowEntryParams struct {
	ID      EntryID `json:"id"`
	History bool    `json:"history,omitempty"`
}

// ShowEntryResult contains an effective entry or oldest-to-newest revisions.
type ShowEntryResult struct {
	Entry   *Entry  `json:"entry,omitempty"`
	History []Entry `json:"history,omitempty"`
}

// SearchEntries returns bounded effective entries matching every filter class.
func (journal *EntryJournal) SearchEntries(params SearchEntriesParams) (SearchEntriesResult, error) {
	canonical, err := canonicalSearchParams(params)
	if err != nil {
		return SearchEntriesResult{}, err
	}

	entries := journal.EffectiveEntries()
	matches := make([]Entry, 0, len(entries))

	for _, entry := range entries {
		if entryMatchesSearch(entry, canonical) {
			matches = append(matches, entry)
		}
	}

	sortEntries(matches, canonical.Order)

	result := SearchEntriesResult{Entries: matches, ReturnedCount: len(matches)}
	if len(matches) > canonical.Limit {
		result.Entries = matches[:canonical.Limit]
		result.ReturnedCount = canonical.Limit
		result.Truncated = true
	}

	return result, nil
}

// ShowEntry returns one effective revision or its complete chain.
func (journal *EntryJournal) ShowEntry(params ShowEntryParams) (ShowEntryResult, error) {
	if err := params.ID.Validate(); err != nil {
		return ShowEntryResult{}, err
	}

	if params.History {
		history, exists := journal.History(params.ID)
		if !exists {
			return ShowEntryResult{}, fmt.Errorf("%w: %q", ErrEntryNotFound, params.ID)
		}

		return ShowEntryResult{History: history}, nil
	}

	entry, exists := journal.Effective(params.ID)
	if !exists {
		return ShowEntryResult{}, fmt.Errorf("%w: %q", ErrEntryNotFound, params.ID)
	}

	return ShowEntryResult{Entry: &entry}, nil
}

func canonicalSearchParams(params SearchEntriesParams) (SearchEntriesParams, error) {
	params.Text = strings.TrimSpace(params.Text)

	params, err := applySearchDefaults(params)
	if err != nil {
		return SearchEntriesParams{}, err
	}

	contexts, err := canonicalizeContexts(params.Contexts)
	if err != nil {
		return SearchEntriesParams{}, fmt.Errorf("search entries: %w", err)
	}

	labels, err := canonicalizeLabels(params.Labels)
	if err != nil {
		return SearchEntriesParams{}, fmt.Errorf("search entries: %w", err)
	}

	params.Contexts = contexts
	params.Labels = labels

	if err := validateSearchEnums(params.Kinds, params.Statuses); err != nil {
		return SearchEntriesParams{}, err
	}

	if err := validateSearchInterval(params.RecordedFrom, params.RecordedTo); err != nil {
		return SearchEntriesParams{}, err
	}

	return params, nil
}

func applySearchDefaults(params SearchEntriesParams) (SearchEntriesParams, error) {
	if params.Order == "" {
		params.Order = EntryOrderDescending
	}

	if params.Order != EntryOrderAscending && params.Order != EntryOrderDescending {
		return SearchEntriesParams{}, fmt.Errorf("search entries: unsupported order %q", params.Order)
	}

	if params.Limit == 0 {
		params.Limit = defaultEntrySearchLimit
	}

	if params.Limit < 1 || params.Limit > maximumEntrySearchLimit {
		return SearchEntriesParams{}, fmt.Errorf("search entries: limit must be between 1 and %d", maximumEntrySearchLimit)
	}

	if len(params.Statuses) == 0 {
		params.Statuses = []EntryStatus{EntryStatusActive}
	}

	return params, nil
}

func validateSearchEnums(kinds []EntryKind, statuses []EntryStatus) error {
	for _, kind := range kinds {
		if !kind.Valid() {
			return fmt.Errorf("search entries: unsupported kind %q", kind)
		}
	}

	for _, status := range statuses {
		if !status.Valid() {
			return fmt.Errorf("search entries: unsupported status %q", status)
		}
	}

	return nil
}

func validateSearchInterval(from, to *time.Time) error {
	if from != nil && to != nil && !from.Before(*to) {
		return fmt.Errorf("search entries: recorded_from must precede recorded_to")
	}

	return nil
}

func entryMatchesSearch(entry Entry, params SearchEntriesParams) bool {
	return matchesEntryText(entry, params.Text) &&
		matchesKind(entry.Kind, params.Kinds) &&
		matchesStatus(entry.Status, params.Statuses) &&
		matchesContexts(entry.Contexts, params.Contexts) &&
		matchesLabels(entry.Labels, params.Labels) &&
		matchesRecordedInterval(entry.RecordedAt, params.RecordedFrom, params.RecordedTo)
}

func matchesEntryText(entry Entry, query string) bool {
	text := strings.ToLower(query)
	if text != "" && !strings.Contains(strings.ToLower(entry.Title), text) && !strings.Contains(strings.ToLower(entry.Body), text) {
		return false
	}

	return true
}

func matchesRecordedInterval(recordedAt time.Time, from, to *time.Time) bool {
	if from != nil && recordedAt.Before(*from) {
		return false
	}

	return to == nil || recordedAt.Before(*to)
}

func matchesKind(value EntryKind, allowed []EntryKind) bool {
	return len(allowed) == 0 || containsComparable(allowed, value)
}

func matchesStatus(value EntryStatus, allowed []EntryStatus) bool {
	return containsComparable(allowed, value)
}

func matchesContexts(actual, required []Context) bool {
	for _, want := range required {
		if !containsComparable(actual, want) {
			return false
		}
	}

	return true
}

func matchesLabels(actual, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}

	for _, label := range actual {
		if containsComparable(allowed, label) {
			return true
		}
	}

	return false
}

func containsComparable[T comparable](values []T, wanted T) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}

	return false
}

func sortEntries(entries []Entry, order EntryOrder) {
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].RecordedAt.Equal(entries[right].RecordedAt) {
			if order == EntryOrderAscending {
				return entries[left].ID < entries[right].ID
			}

			return entries[left].ID > entries[right].ID
		}

		if order == EntryOrderAscending {
			return entries[left].RecordedAt.Before(entries[right].RecordedAt)
		}

		return entries[left].RecordedAt.After(entries[right].RecordedAt)
	})
}

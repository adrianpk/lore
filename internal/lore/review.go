package lore

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ReviewParams selects objective, read-only pruning candidates.
type ReviewParams struct {
	RecordedBefore  *time.Time    `json:"recorded_before,omitempty"`
	VerifiedBefore  *time.Time    `json:"verified_before,omitempty"`
	Statuses        []EntryStatus `json:"statuses,omitempty"`
	Contexts        []Context     `json:"contexts,omitempty"`
	DuplicateTitles bool          `json:"duplicate_titles,omitempty"`
	Limit           int           `json:"limit,omitempty"`
}

// ReviewCandidate is one effective entry and its objective selection reasons.
type ReviewCandidate struct {
	Entry   Entry    `json:"entry"`
	Reasons []string `json:"reasons"`
}

// DuplicateTitleGroup identifies effective entries sharing one canonical title.
type DuplicateTitleGroup struct {
	Title    string    `json:"title"`
	EntryIDs []EntryID `json:"entry_ids"`
}

// ReviewResult contains bounded candidates and duplicate-title groups.
type ReviewResult struct {
	Candidates           []ReviewCandidate     `json:"candidates"`
	DuplicateTitleGroups []DuplicateTitleGroup `json:"duplicate_title_groups,omitempty"`
	ReturnedCount        int                   `json:"returned_count"`
	Truncated            bool                  `json:"truncated"`
}

// Review returns objective effective-entry candidates without changing state.
func (journal *EntryJournal) Review(params ReviewParams) (ReviewResult, error) {
	if !journal.Loaded() {
		return ReviewResult{}, ErrEntryJournalNotLoaded
	}

	canonical, err := canonicalReviewParams(params)
	if err != nil {
		return ReviewResult{}, err
	}

	entries := journal.EffectiveEntries()
	groups := duplicateTitleGroups(entries)
	duplicateIDs := duplicateEntryIDs(groups)
	candidates := make([]ReviewCandidate, 0, len(entries))

	for _, entry := range entries {
		reasons, matches := reviewReasons(entry, canonical, duplicateIDs)
		if matches {
			candidates = append(candidates, ReviewCandidate{Entry: entry, Reasons: reasons})
		}
	}

	sortReviewCandidates(candidates)

	result := ReviewResult{
		Candidates:           candidates,
		DuplicateTitleGroups: selectedDuplicateGroups(groups, candidates),
		ReturnedCount:        len(candidates),
	}
	if len(candidates) > canonical.Limit {
		result.Candidates = candidates[:canonical.Limit]
		result.ReturnedCount = canonical.Limit
		result.Truncated = true
		result.DuplicateTitleGroups = selectedDuplicateGroups(groups, result.Candidates)
	}

	return result, nil
}

func canonicalReviewParams(params ReviewParams) (ReviewParams, error) {
	if !hasReviewSelector(params) {
		return ReviewParams{}, fmt.Errorf("review entries: at least one objective selector is required")
	}

	if params.Limit == 0 {
		params.Limit = defaultEntrySearchLimit
	}

	if params.Limit < 1 || params.Limit > maximumEntrySearchLimit {
		return ReviewParams{}, fmt.Errorf("review entries: limit must be between 1 and %d", maximumEntrySearchLimit)
	}

	if err := validateSearchEnums(nil, params.Statuses); err != nil {
		return ReviewParams{}, err
	}

	times := []struct {
		name  string
		value *time.Time
	}{
		{name: "recorded_before", value: params.RecordedBefore},
		{name: "verified_before", value: params.VerifiedBefore},
	}

	for _, candidateTime := range times {
		if candidateTime.value != nil {
			if err := validateUTCTime("review "+candidateTime.name, *candidateTime.value); err != nil {
				return ReviewParams{}, err
			}
		}
	}

	contexts, err := canonicalizeContexts(params.Contexts)
	if err != nil {
		return ReviewParams{}, fmt.Errorf("review entries: %w", err)
	}

	params.Contexts = contexts

	return params, nil
}

func hasReviewSelector(params ReviewParams) bool {
	return params.RecordedBefore != nil ||
		params.VerifiedBefore != nil ||
		len(params.Statuses) > 0 ||
		len(params.Contexts) > 0 ||
		params.DuplicateTitles
}

func reviewReasons(entry Entry, params ReviewParams, duplicateIDs map[EntryID]struct{}) ([]string, bool) {
	timeReasons, matches := reviewTimeReasons(entry, params)
	if !matches {
		return nil, false
	}

	categoryReasons, matches := reviewCategoryReasons(entry, params, duplicateIDs)
	if !matches {
		return nil, false
	}

	return append(timeReasons, categoryReasons...), true
}

func reviewTimeReasons(entry Entry, params ReviewParams) ([]string, bool) {
	var reasons []string

	if params.RecordedBefore != nil {
		if !entry.RecordedAt.Before(*params.RecordedBefore) {
			return nil, false
		}

		reasons = append(reasons, "recorded_before")
	}

	if params.VerifiedBefore != nil {
		if entry.VerifiedAt == nil || !entry.VerifiedAt.Before(*params.VerifiedBefore) {
			return nil, false
		}

		reasons = append(reasons, "verified_before")
	}

	return reasons, true
}

func reviewCategoryReasons(entry Entry, params ReviewParams, duplicateIDs map[EntryID]struct{}) ([]string, bool) {
	var reasons []string

	if len(params.Statuses) > 0 {
		if !matchesStatus(entry.Status, params.Statuses) {
			return nil, false
		}

		reasons = append(reasons, "status")
	}

	if len(params.Contexts) > 0 {
		if !matchesContexts(entry.Contexts, params.Contexts) {
			return nil, false
		}

		reasons = append(reasons, "context")
	}

	if params.DuplicateTitles {
		if _, exists := duplicateIDs[entry.ID]; !exists {
			return nil, false
		}

		reasons = append(reasons, "duplicate_title")
	}

	return reasons, true
}

func duplicateTitleGroups(entries []Entry) []DuplicateTitleGroup {
	byTitle := make(map[string][]EntryID)

	for _, entry := range entries {
		title := canonicalReviewTitle(entry.Title)
		byTitle[title] = append(byTitle[title], entry.ID)
	}

	groups := make([]DuplicateTitleGroup, 0)

	for title, ids := range byTitle {
		if len(ids) > 1 {
			sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })

			groups = append(groups, DuplicateTitleGroup{Title: title, EntryIDs: ids})
		}
	}

	sort.Slice(groups, func(left, right int) bool { return groups[left].Title < groups[right].Title })

	return groups
}

func canonicalReviewTitle(title string) string {
	return strings.ToLower(strings.Join(strings.Fields(title), " "))
}

func duplicateEntryIDs(groups []DuplicateTitleGroup) map[EntryID]struct{} {
	ids := make(map[EntryID]struct{})

	for _, group := range groups {
		for _, id := range group.EntryIDs {
			ids[id] = struct{}{}
		}
	}

	return ids
}

func sortReviewCandidates(candidates []ReviewCandidate) {
	sort.Slice(candidates, func(left, right int) bool {
		leftEntry := candidates[left].Entry
		rightEntry := candidates[right].Entry

		if leftEntry.RecordedAt.Equal(rightEntry.RecordedAt) {
			return leftEntry.ID > rightEntry.ID
		}

		return leftEntry.RecordedAt.After(rightEntry.RecordedAt)
	})
}

func selectedDuplicateGroups(groups []DuplicateTitleGroup, candidates []ReviewCandidate) []DuplicateTitleGroup {
	selected := make(map[EntryID]struct{}, len(candidates))

	for _, candidate := range candidates {
		selected[candidate.Entry.ID] = struct{}{}
	}

	result := make([]DuplicateTitleGroup, 0)

	for _, group := range groups {
		var ids []EntryID

		for _, id := range group.EntryIDs {
			if _, exists := selected[id]; exists {
				ids = append(ids, id)
			}
		}

		if len(ids) > 1 {
			result = append(result, DuplicateTitleGroup{Title: group.Title, EntryIDs: ids})
		}
	}

	return result
}

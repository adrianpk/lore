package lore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultQueryLimit = 20
	maximumQueryLimit = 1000
)

// ErrQueryInvalid identifies an invalid retrieval filter.
var ErrQueryInvalid = errors.New("query is invalid")

// ErrCommitAmbiguous identifies an abbreviated commit with several matches.
var ErrCommitAmbiguous = errors.New("commit is ambiguous")

// QueryOrder selects chronological result direction.
type QueryOrder string

const (
	// QueryAscending orders older events first.
	QueryAscending QueryOrder = "ascending"
	// QueryDescending orders newer events first.
	QueryDescending QueryOrder = "descending"
)

// Query selects a bounded effective-activity view.
type Query struct {
	Types        []ActivityType `json:"types,omitempty"`
	Scope        string         `json:"scope,omitempty"`
	Commit       string         `json:"commit,omitempty"`
	PullRequest  string         `json:"pull_request,omitempty"`
	Harness      string         `json:"harness,omitempty"`
	Model        string         `json:"model,omitempty"`
	Text         string         `json:"text,omitempty"`
	Source       ProvenanceType `json:"source,omitempty"`
	OccurredFrom *time.Time     `json:"occurred_from,omitempty"`
	OccurredTo   *time.Time     `json:"occurred_to,omitempty"`
	RecordedFrom *time.Time     `json:"recorded_from,omitempty"`
	RecordedTo   *time.Time     `json:"recorded_to,omitempty"`
	DurationMin  int            `json:"duration_min,omitempty"`
	DurationMax  int            `json:"duration_max,omitempty"`
	Limit        int            `json:"limit"`
	Order        QueryOrder     `json:"order"`
}

// QueryResult contains bounded effective activities and truncation metadata.
type QueryResult struct {
	Query         Query                `json:"query"`
	Events        []ActivityProjection `json:"events"`
	ReturnedCount int                  `json:"returned_count"`
	Truncated     bool                 `json:"truncated"`
}

// Search returns effective activities matching one normalized bounded query.
func (journal *Journal) Search(ctx context.Context, query Query) (QueryResult, error) {
	normalized, matches, err := journal.findEffectiveActivities(ctx, query)
	if err != nil {
		return QueryResult{}, err
	}

	projections := make([]ActivityProjection, 0, len(matches))
	for _, event := range matches {
		projections = append(projections, projectActivity(event))
	}

	return boundedQueryResult(normalized, projections), nil
}

func (journal *Journal) findEffectiveActivities(ctx context.Context, query Query) (Query, []Activity, error) {
	normalized, err := normalizeQuery(query)
	if err != nil {
		return Query{}, nil, err
	}

	journal.mu.RLock()
	defer journal.mu.RUnlock()

	if !journal.loaded {
		return Query{}, nil, ErrJournalNotLoaded
	}

	matches, err := journal.queryMatches(ctx, normalized)
	if err != nil {
		return Query{}, nil, err
	}

	sortQueryActivities(matches, normalized.Order)

	return normalized, matches, nil
}

func (journal *Journal) queryMatches(ctx context.Context, query Query) ([]Activity, error) {
	matches := make([]Activity, 0)
	commits := make(map[string]struct{})

	for _, id := range journal.order {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		event := journal.events[id]

		root := journal.roots[id]
		if journal.effective[root] != id || !activityMatchesQuery(event, query) {
			continue
		}

		matches = append(matches, event)
		if query.Commit != "" && event.Properties != nil {
			commits[event.Properties.ObjectID] = struct{}{}
		}
	}

	if len(commits) > 1 {
		return nil, fmt.Errorf("%w: %q", ErrCommitAmbiguous, query.Commit)
	}

	return matches, nil
}

func normalizeQuery(query Query) (Query, error) {
	query.Scope = strings.TrimSpace(query.Scope)
	query.Commit = strings.ToLower(strings.TrimSpace(query.Commit))
	query.PullRequest = normalizeQueryText(query.PullRequest)
	query.Harness = normalizeQueryText(query.Harness)
	query.Model = normalizeQueryText(query.Model)
	query.Text = normalizeQueryText(query.Text)

	if query.Limit == 0 {
		query.Limit = defaultQueryLimit
	}

	if query.Limit < 1 || query.Limit > maximumQueryLimit {
		return Query{}, fmt.Errorf("%w: limit must be between 1 and %d", ErrQueryInvalid, maximumQueryLimit)
	}

	if query.Order == "" {
		query.Order = QueryDescending
	}

	if query.Order != QueryAscending && query.Order != QueryDescending {
		return Query{}, fmt.Errorf("%w: unsupported order %q", ErrQueryInvalid, query.Order)
	}

	if err := normalizeQueryFilters(&query); err != nil {
		return Query{}, err
	}

	return query, nil
}

func normalizeQueryFilters(query *Query) error {
	if err := validateCommitPrefix(query.Commit); err != nil {
		return err
	}

	if err := normalizeActivityTypes(query); err != nil {
		return err
	}

	if query.Source != "" && !query.Source.Valid() {
		return fmt.Errorf("%w: unsupported source %q", ErrQueryInvalid, query.Source)
	}

	normalizeQueryTimes(query)

	return validateQueryRanges(*query)
}

func normalizeActivityTypes(query *Query) error {
	seen := make(map[ActivityType]struct{})

	for _, activityType := range query.Types {
		if !activityType.Valid() {
			return fmt.Errorf("%w: unsupported activity type %q", ErrQueryInvalid, activityType)
		}

		seen[activityType] = struct{}{}
	}

	query.Types = query.Types[:0]
	for activityType := range seen {
		query.Types = append(query.Types, activityType)
	}

	sort.Slice(query.Types, func(left, right int) bool {
		return query.Types[left] < query.Types[right]
	})

	return nil
}

// Valid reports whether the activity type is canonical.
func (activityType ActivityType) Valid() bool {
	switch activityType {
	case ActivityTypeCommit, ActivityTypePullRequest, ActivityTypeWorklog, ActivityTypeNote:
		return true
	default:
		return false
	}
}

func normalizeQueryTimes(query *Query) {
	query.OccurredFrom = utcTimePointer(query.OccurredFrom)
	query.OccurredTo = utcTimePointer(query.OccurredTo)
	query.RecordedFrom = utcTimePointer(query.RecordedFrom)
	query.RecordedTo = utcTimePointer(query.RecordedTo)
}

func utcTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	utc := value.UTC()

	return &utc
}

func validateQueryRanges(query Query) error {
	if invalidTimeRange(query.OccurredFrom, query.OccurredTo) {
		return fmt.Errorf("%w: occurred range is empty or reversed", ErrQueryInvalid)
	}

	if invalidTimeRange(query.RecordedFrom, query.RecordedTo) {
		return fmt.Errorf("%w: recorded range is empty or reversed", ErrQueryInvalid)
	}

	if query.DurationMin < 0 || query.DurationMax < 0 {
		return fmt.Errorf("%w: duration bounds must not be negative", ErrQueryInvalid)
	}

	if query.DurationMin != 0 && query.DurationMax != 0 && query.DurationMin > query.DurationMax {
		return fmt.Errorf("%w: duration range is reversed", ErrQueryInvalid)
	}

	return nil
}

func invalidTimeRange(from, to *time.Time) bool {
	return from != nil && to != nil && !from.Before(*to)
}

func validateCommitPrefix(commit string) error {
	if commit == "" {
		return nil
	}

	if len(commit) > 64 {
		return fmt.Errorf("%w: commit prefix is too long", ErrQueryInvalid)
	}

	for _, character := range commit {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return fmt.Errorf("%w: commit prefix must be lowercase hexadecimal", ErrQueryInvalid)
		}
	}

	return nil
}

func activityMatchesQuery(event Activity, query Query) bool {
	return matchesActivityType(event, query.Types) &&
		matchesScope(event, query.Scope) &&
		matchesCommit(event, query.Commit) &&
		matchesPullRequest(event, query.PullRequest) &&
		matchesProvenance(event, query) &&
		matchesDuration(event, query.DurationMin, query.DurationMax) &&
		matchesText(event, query.Text) &&
		withinTimeRange(event.OccurredAt, query.OccurredFrom, query.OccurredTo) &&
		withinTimeRange(event.RecordedAt, query.RecordedFrom, query.RecordedTo)
}

func matchesActivityType(event Activity, activityTypes []ActivityType) bool {
	if len(activityTypes) == 0 {
		return true
	}

	for _, activityType := range activityTypes {
		if event.Type == activityType {
			return true
		}
	}

	return false
}

func matchesScope(event Activity, scope string) bool {
	if scope == "" {
		return true
	}

	return event.Scope != nil && (strings.EqualFold(event.Scope.Name, scope) || strings.EqualFold(event.Scope.Identity, scope))
}

func matchesCommit(event Activity, commit string) bool {
	if commit == "" {
		return true
	}

	return event.Type == ActivityTypeCommit && event.Properties != nil && strings.HasPrefix(event.Properties.ObjectID, commit)
}

func matchesPullRequest(event Activity, pullRequest string) bool {
	if pullRequest == "" {
		return true
	}

	if event.Type != ActivityTypePullRequest || event.Properties == nil {
		return false
	}

	return strings.Contains(normalizeQueryText(pullRequestIdentifier(*event.Properties)), pullRequest)
}

func matchesProvenance(event Activity, query Query) bool {
	if query.Source == "" && query.Harness == "" && query.Model == "" {
		return true
	}

	if event.Provenance == nil {
		return false
	}

	return (query.Source == "" || event.Provenance.Type == query.Source) &&
		(query.Harness == "" || normalizeQueryText(event.Provenance.Harness) == query.Harness) &&
		(query.Model == "" || normalizeQueryText(event.Provenance.Model) == query.Model)
}

func matchesDuration(event Activity, minimum, maximum int) bool {
	if minimum == 0 && maximum == 0 {
		return true
	}

	if event.Type != ActivityTypeWorklog || event.Properties == nil {
		return false
	}

	duration := event.Properties.DurationMinutes

	return (minimum == 0 || duration >= minimum) && (maximum == 0 || duration <= maximum)
}

func matchesText(event Activity, text string) bool {
	if text == "" {
		return true
	}

	for _, value := range activitySearchValues(event) {
		if strings.Contains(normalizeQueryText(value), text) {
			return true
		}
	}

	return false
}

func activitySearchValues(event Activity) []string {
	values := []string{event.Summary, event.CorrectionReason}
	if event.Scope != nil {
		values = append(values, event.Scope.Name, event.Scope.Identity)
	}

	if event.Properties != nil {
		values = append(values, propertySearchValues(*event.Properties)...)
	}

	for _, reference := range event.References {
		values = append(values, reference.ScopeIdentity, reference.ObjectID, reference.URL)
	}

	if event.Provenance != nil {
		values = append(values, event.Provenance.Harness, event.Provenance.Model)
	}

	return values
}

func propertySearchValues(properties ActivityProperties) []string {
	return []string{
		properties.ObjectID,
		properties.Subject,
		properties.Forge,
		properties.Repository,
		strconv.Itoa(properties.Number),
		string(properties.Action),
		properties.URL,
	}
}

func withinTimeRange(value time.Time, from, to *time.Time) bool {
	return (from == nil || !value.Before(*from)) && (to == nil || value.Before(*to))
}

func normalizeQueryText(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func sortQueryActivities(events []Activity, order QueryOrder) {
	sort.Slice(events, func(left, right int) bool {
		comparison := compareActivityTimes(events[left], events[right])
		if order == QueryAscending {
			return comparison < 0
		}

		return comparison > 0
	})
}

func compareActivityTimes(left, right Activity) int {
	if comparison := left.OccurredAt.Compare(right.OccurredAt); comparison != 0 {
		return comparison
	}

	if comparison := left.RecordedAt.Compare(right.RecordedAt); comparison != 0 {
		return comparison
	}

	return strings.Compare(string(left.ID), string(right.ID))
}

func boundedQueryResult(query Query, matches []ActivityProjection) QueryResult {
	result := QueryResult{Query: query, Events: make([]ActivityProjection, 0, min(query.Limit, len(matches)))}

	for index, event := range matches {
		if index >= query.Limit {
			result.Truncated = true

			break
		}

		candidate := result
		candidate.Events = append(append([]ActivityProjection(nil), result.Events...), event)
		candidate.ReturnedCount = len(candidate.Events)
		candidate.Truncated = index+1 < len(matches)

		if !queryResponseFits(candidate) {
			result.Truncated = true

			break
		}

		result = candidate
	}

	result.ReturnedCount = len(result.Events)

	return result
}

func projectActivity(event Activity) ActivityProjection {
	return ActivityProjection{
		ID:              event.ID,
		Type:            event.Type,
		OccurredAt:      event.OccurredAt,
		RecordedAt:      event.RecordedAt,
		ScopeName:       activityScopeName(event),
		Summary:         event.Summary,
		DurationMinutes: activityDuration(event),
		Identifier:      activityIdentifier(event),
	}
}

func queryResponseFits(result QueryResult) bool {
	response := SuccessResponse(result)
	encoded, err := json.Marshal(response)

	return err == nil && len(encoded)+1 <= MaxResponseSize
}

func pullRequestIdentifier(properties ActivityProperties) string {
	return fmt.Sprintf("%s/%s#%d:%s", properties.Forge, properties.Repository, properties.Number, properties.Action)
}

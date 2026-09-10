package lore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const reportDateLayout = "2006-01-02"

// ErrReportInvalid identifies an invalid report interval or timezone.
var ErrReportInvalid = errors.New("report is invalid")

// ReportParams selects an explicit local-calendar occurrence interval.
type ReportParams struct {
	From     string         `json:"from"`
	To       string         `json:"to"`
	Timezone string         `json:"timezone,omitempty"`
	Types    []ActivityType `json:"types,omitempty"`
	Limit    int            `json:"limit,omitempty"`
}

// ReportInterval describes the effective local-calendar interval.
type ReportInterval struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Timezone string `json:"timezone"`
}

// ReportScope groups effective activities for one scope.
type ReportScope struct {
	Name           string               `json:"name"`
	Identity       string               `json:"identity,omitempty"`
	Events         []ActivityProjection `json:"events"`
	EventCount     int                  `json:"event_count"`
	WorklogMinutes int                  `json:"worklog_minutes"`
}

// ReportDay groups scopes by local occurrence date.
type ReportDay struct {
	Date           string        `json:"date"`
	Scopes         []ReportScope `json:"scopes"`
	EventCount     int           `json:"event_count"`
	WorklogMinutes int           `json:"worklog_minutes"`
}

// ReportResult contains one bounded grouped activity report.
type ReportResult struct {
	Interval       ReportInterval `json:"interval"`
	Days           []ReportDay    `json:"days"`
	ReturnedCount  int            `json:"returned_count"`
	EventCount     int            `json:"event_count"`
	WorklogMinutes int            `json:"worklog_minutes"`
	Truncated      bool           `json:"truncated"`
}

type normalizedReport struct {
	interval ReportInterval
	location *time.Location
	from     time.Time
	to       time.Time
	types    []ActivityType
	limit    int
}

// Report returns effective activities grouped by local date and scope.
func (journal *Journal) Report(ctx context.Context, params ReportParams) (ReportResult, error) {
	normalized, err := normalizeReport(params)
	if err != nil {
		return ReportResult{}, err
	}

	_, events, err := journal.findEffectiveActivities(ctx, Query{
		Types:        normalized.types,
		OccurredFrom: &normalized.from,
		OccurredTo:   &normalized.to,
		Limit:        normalized.limit,
		Order:        QueryAscending,
	})
	if err != nil {
		return ReportResult{}, err
	}

	truncated := len(events) > normalized.limit
	if truncated {
		events = events[:normalized.limit]
	}

	return boundedReportResult(normalized, events, truncated), nil
}

func normalizeReport(params ReportParams) (normalizedReport, error) {
	location, err := reportLocation(strings.TrimSpace(params.Timezone))
	if err != nil {
		return normalizedReport{}, err
	}

	from, err := parseReportDate("from", params.From, location)
	if err != nil {
		return normalizedReport{}, err
	}

	to, err := parseReportDate("to", params.To, location)
	if err != nil {
		return normalizedReport{}, err
	}

	if !from.Before(to) {
		return normalizedReport{}, fmt.Errorf("%w: from must be before to", ErrReportInvalid)
	}

	types, err := normalizedReportTypes(params.Types)
	if err != nil {
		return normalizedReport{}, err
	}

	limit, err := normalizedReportLimit(params.Limit)
	if err != nil {
		return normalizedReport{}, err
	}

	return normalizedReport{
		interval: ReportInterval{
			From:     from.In(location).Format(reportDateLayout),
			To:       to.In(location).Format(reportDateLayout),
			Timezone: location.String(),
		},
		location: location,
		from:     from.UTC(),
		to:       to.UTC(),
		types:    types,
		limit:    limit,
	}, nil
}

func normalizedReportTypes(types []ActivityType) ([]ActivityType, error) {
	query, err := normalizeQuery(Query{Types: types})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrReportInvalid, err)
	}

	return query.Types, nil
}

func normalizedReportLimit(limit int) (int, error) {
	if limit == 0 {
		return maximumQueryLimit, nil
	}

	if limit < 1 || limit > maximumQueryLimit {
		return 0, fmt.Errorf("%w: limit must be between 1 and %d", ErrReportInvalid, maximumQueryLimit)
	}

	return limit, nil
}

func reportLocation(name string) (*time.Location, error) {
	if name == "" {
		return time.Local, nil
	}

	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("%w: load timezone %q: %w", ErrReportInvalid, name, err)
	}

	return location, nil
}

func parseReportDate(name, value string, location *time.Location) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("%w: %s date is required", ErrReportInvalid, name)
	}

	parsed, err := time.ParseInLocation(reportDateLayout, trimmed, location)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: parse %s date: %w", ErrReportInvalid, name, err)
	}

	return parsed, nil
}

func boundedReportResult(report normalizedReport, events []Activity, sourceTruncated bool) ReportResult {
	result := buildReportResult(report, events, sourceTruncated)
	if reportResponseFits(result) {
		return result
	}

	low := 0
	high := len(events)

	for low < high {
		middle := low + (high-low+1)/2

		candidate := buildReportResult(report, events[:middle], true)
		if reportResponseFits(candidate) {
			low = middle
		} else {
			high = middle - 1
		}
	}

	return buildReportResult(report, events[:low], true)
}

func buildReportResult(report normalizedReport, events []Activity, truncated bool) ReportResult {
	result := ReportResult{
		Interval:      report.interval,
		Days:          groupReportActivities(events, report.location),
		ReturnedCount: len(events),
		EventCount:    len(events),
		Truncated:     truncated,
	}
	for _, event := range events {
		result.WorklogMinutes += activityDuration(event)
	}

	return result
}

func groupReportActivities(events []Activity, location *time.Location) []ReportDay {
	days := make([]ReportDay, 0)
	dayIndexes := make(map[string]int)
	scopeIndexes := make(map[string]map[string]int)

	for _, event := range events {
		date := event.OccurredAt.In(location).Format(reportDateLayout)
		dayIndex := ensureReportDay(&days, dayIndexes, scopeIndexes, date)
		addReportActivity(&days[dayIndex], scopeIndexes[date], event)
	}

	for index := range days {
		sort.Slice(days[index].Scopes, func(left, right int) bool {
			leftScope := days[index].Scopes[left]
			rightScope := days[index].Scopes[right]

			if leftScope.Name == rightScope.Name {
				return leftScope.Identity < rightScope.Identity
			}

			return leftScope.Name < rightScope.Name
		})
	}

	return days
}

func ensureReportDay(days *[]ReportDay, dayIndexes map[string]int, scopeIndexes map[string]map[string]int, date string) int {
	if index, ok := dayIndexes[date]; ok {
		return index
	}

	index := len(*days)
	dayIndexes[date] = index
	scopeIndexes[date] = make(map[string]int)
	*days = append(*days, ReportDay{Date: date, Scopes: make([]ReportScope, 0)})

	return index
}

func addReportActivity(day *ReportDay, indexes map[string]int, event Activity) {
	name, identity := activityReportScope(event)
	key := name + "\x00" + identity

	index, ok := indexes[key]
	if !ok {
		index = len(day.Scopes)
		indexes[key] = index

		day.Scopes = append(day.Scopes, ReportScope{Name: name, Identity: identity, Events: make([]ActivityProjection, 0)})
	}

	duration := activityDuration(event)
	scope := &day.Scopes[index]
	scope.Events = append(scope.Events, projectActivity(event))
	scope.EventCount++
	scope.WorklogMinutes += duration
	day.EventCount++
	day.WorklogMinutes += duration
}

func activityReportScope(event Activity) (string, string) {
	if event.Scope == nil {
		return "Unscoped", ""
	}

	return event.Scope.Name, event.Scope.Identity
}

func activityDuration(event Activity) int {
	if event.Type != ActivityTypeWorklog || event.Properties == nil {
		return 0
	}

	return event.Properties.DurationMinutes
}

func reportResponseFits(result ReportResult) bool {
	response := SuccessResponse(result)
	encoded, err := json.Marshal(response)

	return err == nil && len(encoded)+1 <= MaxResponseSize
}

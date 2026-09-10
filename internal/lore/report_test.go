package lore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestJournalReportGroupsCrossMonthEventsInSuppliedTimezone(t *testing.T) {
	t.Parallel()

	journal := newCrossMonthReportJournal(t)

	result, err := journal.Report(context.Background(), ReportParams{
		From:     "2026-08-31",
		To:       "2026-09-02",
		Timezone: "Europe/Warsaw",
	})
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}

	assertCrossMonthReport(t, result)
}

func assertCrossMonthReport(t *testing.T, result ReportResult) {
	t.Helper()

	if result.Interval.Timezone != "Europe/Warsaw" || result.ReturnedCount != 3 || result.Truncated {
		t.Fatalf("Report() result = %#v", result)
	}

	if len(result.Days) != 2 || result.Days[0].Date != "2026-08-31" || result.Days[1].Date != "2026-09-01" {
		t.Fatalf("report days = %#v", result.Days)
	}

	septemberScopes := result.Days[1].Scopes
	if len(septemberScopes) != 2 || septemberScopes[0].Name != "alpha" || septemberScopes[1].Name != "zeta" {
		t.Fatalf("September scopes = %#v", septemberScopes)
	}
}

func TestJournalReportUsesInclusiveFromAndExclusiveTo(t *testing.T) {
	t.Parallel()

	journal := newCrossMonthReportJournal(t)

	result, err := journal.Report(context.Background(), ReportParams{
		From:     "2026-09-01",
		To:       "2026-09-02",
		Timezone: "Europe/Warsaw",
	})
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}

	if result.ReturnedCount != 2 || len(result.Days) != 1 || result.Days[0].Date != "2026-09-01" {
		t.Fatalf("Report() result = %#v", result)
	}
}

func TestJournalReportSumsOnlyExplicitWorklogMinutes(t *testing.T) {
	t.Parallel()

	journal := NewJournal(testJournalConfig(t))
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("load journal: %v", err)
	}

	worklog := validWorklogActivity()
	worklog.RecordedAt = time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)
	worklog.OccurredAt = worklog.RecordedAt
	worklog.ID = nextEventID(t, worklog.RecordedAt, 41)
	note := validNoteActivity()
	note.RecordedAt = time.Date(2026, 9, 3, 11, 0, 0, 0, time.UTC)
	note.OccurredAt = note.RecordedAt
	note.ID = nextEventID(t, note.RecordedAt, 42)

	appendJournalEvent(t, journal, worklog)
	appendJournalEvent(t, journal, note)

	result, err := journal.Report(context.Background(), ReportParams{From: "2026-09-03", To: "2026-09-04", Timezone: "UTC"})
	if err != nil {
		t.Fatalf("Report() error = %v", err)
	}

	assertExplicitWorklogTotals(t, result)
	assertCompactReportProjection(t, result)
}

func assertExplicitWorklogTotals(t *testing.T, result ReportResult) {
	t.Helper()

	if result.EventCount != 2 || result.WorklogMinutes != 240 || result.ReturnedCount != 2 {
		t.Fatalf("report totals = %#v", result)
	}

	if len(result.Days) != 1 || len(result.Days[0].Scopes) != 2 {
		t.Fatalf("report groups = %#v", result.Days)
	}
}

func assertCompactReportProjection(t *testing.T, result ReportResult) {
	t.Helper()

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}

	if strings.Contains(string(encoded), "properties") || !strings.Contains(string(encoded), "duration_minutes") {
		t.Fatalf("report projection = %s", encoded)
	}
}

func TestJournalReportDefaultsToLocalTimezoneAndMaximumLimit(t *testing.T) {
	t.Parallel()

	normalized, err := normalizeReport(ReportParams{From: "2026-08-01", To: "2026-08-02"})
	if err != nil {
		t.Fatalf("normalizeReport() error = %v", err)
	}

	if normalized.location != time.Local || normalized.interval.Timezone != time.Local.String() {
		t.Fatalf("normalized timezone = %#v", normalized.interval)
	}

	if normalized.limit != maximumQueryLimit {
		t.Fatalf("normalized limit = %d, want %d", normalized.limit, maximumQueryLimit)
	}
}

func TestJournalReportValidatesIntervalTimezoneAndLimit(t *testing.T) {
	t.Parallel()

	journal := newCrossMonthReportJournal(t)
	tests := []ReportParams{
		{},
		{From: "2026-09-01", To: "2026-09-01", Timezone: "UTC"},
		{From: "2026-09-01", To: "2026-09-02", Timezone: "Missing/Timezone"},
		{From: "2026-09-01", To: "2026-09-02", Timezone: "UTC", Limit: maximumQueryLimit + 1},
	}

	for _, params := range tests {
		if _, err := journal.Report(context.Background(), params); !errors.Is(err, ErrReportInvalid) {
			t.Fatalf("Report(%#v) error = %v, want ErrReportInvalid", params, err)
		}
	}
}

func TestJournalReportTruncatesByCountAndEncodedSize(t *testing.T) {
	t.Parallel()

	journal := newCrossMonthReportJournal(t)

	limited, err := journal.Report(context.Background(), ReportParams{
		From:     "2026-08-31",
		To:       "2026-09-02",
		Timezone: "Europe/Warsaw",
		Limit:    1,
	})
	if err != nil {
		t.Fatalf("limited Report() error = %v", err)
	}

	if limited.ReturnedCount != 1 || !limited.Truncated {
		t.Fatalf("limited report = %#v", limited)
	}

	const matchCount = 10

	event := validMilestoneEvent()
	event.Summary = strings.Repeat("x", 1<<20)

	events := make([]Event, matchCount)
	for index := range events {
		events[index] = event
	}

	normalized, err := normalizeReport(ReportParams{From: "2026-08-01", To: "2026-08-02", Timezone: "UTC"})
	if err != nil {
		t.Fatalf("normalize report: %v", err)
	}

	bounded := boundedReportResult(normalized, events, false)

	encoded, err := json.Marshal(SuccessResponse(bounded))
	if err != nil {
		t.Fatalf("marshal bounded report: %v", err)
	}

	if !bounded.Truncated || bounded.ReturnedCount >= matchCount || len(encoded)+1 > MaxResponseSize {
		t.Fatalf("bounded report count = %d, truncated = %v, bytes = %d", bounded.ReturnedCount, bounded.Truncated, len(encoded)+1)
	}
}

func newCrossMonthReportJournal(t *testing.T) *Journal {
	t.Helper()

	journal := NewJournal(testJournalConfig(t))
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("load journal: %v", err)
	}

	first := distinctMilestoneEvent(t, 11)
	first.OccurredAt = time.Date(2026, 8, 31, 21, 30, 0, 0, time.UTC)
	first.RecordedAt = time.Date(2026, 8, 31, 21, 30, 0, 0, time.UTC)
	first.ID = nextEventID(t, first.RecordedAt, 11)
	first.Scope = reportScope("middle")

	second := distinctMilestoneEvent(t, 12)
	second.OccurredAt = time.Date(2026, 8, 31, 22, 30, 0, 0, time.UTC)
	second.RecordedAt = time.Date(2026, 8, 31, 22, 30, 0, 0, time.UTC)
	second.ID = nextEventID(t, second.RecordedAt, 12)
	second.Scope = reportScope("zeta")

	third := distinctMilestoneEvent(t, 13)
	third.OccurredAt = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	third.RecordedAt = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	third.ID = nextEventID(t, third.RecordedAt, 13)
	third.Scope = reportScope("alpha")

	for _, event := range []Event{first, second, third} {
		appendJournalEvent(t, journal, event)
	}

	return journal
}

func reportScope(name string) *ActivityScope {
	return &ActivityScope{
		Type:     ScopeTypeRepository,
		Name:     name,
		Identity: "https://example.test/team/" + name + ".git",
	}
}

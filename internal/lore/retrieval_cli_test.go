package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestParseSearchOptions(t *testing.T) {
	t.Parallel()

	options, err := parseSearchOptions([]string{
		"--query", "runtime",
		"--scope", "example",
		"--pull-request", "example#42:merged",
		"--source", "human",
		"--type", "commit,note",
		"--occurred-from", "2026-07-31T22:00:00Z",
		"--recorded-from", "2026-08-01T00:00:00+02:00",
		"--duration-min", "30",
		"--duration-max", "240",
		"--limit", "12",
		"--order", "ascending",
		"--json",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseSearchOptions() error = %v", err)
	}

	wantTime := time.Date(2026, 8, 1, 0, 0, 0, 0, time.FixedZone("offset", 2*60*60))

	assertParsedSearchCore(t, options)
	assertParsedSearchFilters(t, options, wantTime)
}

func assertParsedSearchCore(t *testing.T, options searchOptions) {
	t.Helper()

	if !options.json || options.query.Text != "runtime" || options.query.Scope != "example" {
		t.Fatalf("parseSearchOptions() = %#v", options)
	}

	if options.query.PullRequest != "example#42:merged" || options.query.Source != ProvenanceTypeHuman {
		t.Fatalf("parseSearchOptions() = %#v", options)
	}

	if len(options.query.Types) != 2 || options.query.Limit != 12 || options.query.Order != QueryAscending {
		t.Fatalf("parseSearchOptions() = %#v", options)
	}
}

func assertParsedSearchFilters(t *testing.T, options searchOptions, wantTime time.Time) {
	t.Helper()

	if !options.query.OccurredFrom.Equal(wantTime) || !options.query.RecordedFrom.Equal(wantTime) {
		t.Fatalf("parseSearchOptions() = %#v", options)
	}

	if options.query.DurationMin != 30 || options.query.DurationMax != 240 {
		t.Fatalf("parseSearchOptions() = %#v", options)
	}
}

func TestParseRetrievalOptionsRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		parse func() error
	}{
		{
			name: "search timestamp",
			parse: func() error {
				_, err := parseSearchOptions([]string{"--recorded-from", "today"}, &bytes.Buffer{})

				return err
			},
		},
		{
			name: "show commit",
			parse: func() error {
				_, err := parseShowOptions(nil, &bytes.Buffer{})

				return err
			},
		},
		{
			name: "correction reason",
			parse: func() error {
				_, err := parseCorrectOptions([]string{"--target", "01K1Z8V7BMMN6W7Y6R8QJHA9SS"}, &bytes.Buffer{})

				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := test.parse(); err == nil {
				t.Fatal("expected parse error")
			}
		})
	}
}

func TestParseCorrectOptionsDistinguishesAbsentAndEmptySummary(t *testing.T) {
	t.Parallel()

	base := []string{
		"--target", "01K1Z8V7BMMN6W7Y6R8QJHA9SS",
		"--reason", "Clarify behavior",
	}

	absent, err := parseCorrectOptions(base, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse absent note: %v", err)
	}

	empty, err := parseCorrectOptions(append(base, "--summary", ""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse empty note: %v", err)
	}

	if absent.params.ReplacementSummary != nil || empty.params.ReplacementSummary == nil || *empty.params.ReplacementSummary != "" {
		t.Fatalf("replacement summaries = %#v, %#v", absent.params.ReplacementSummary, empty.params.ReplacementSummary)
	}
}

func TestParseCorrectOptionsAcceptsOccurrenceAndDuration(t *testing.T) {
	t.Parallel()

	options, err := parseCorrectOptions([]string{
		"--target", "01K1Z8V7BMMN6W7Y6R8QJHA9SS",
		"--reason", "Correct timesheet facts",
		"--at", "2026-08-01T08:00:00+02:00",
		"--duration", "90",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseCorrectOptions() error = %v", err)
	}

	wantTime := time.Date(2026, 8, 1, 8, 0, 0, 0, time.FixedZone("offset", 2*60*60))
	if !options.params.ReplacementOccurredAt.Equal(wantTime) {
		t.Fatalf("parseCorrectOptions() = %#v", options)
	}

	if options.params.ReplacementDurationMinutes == nil || *options.params.ReplacementDurationMinutes != 90 {
		t.Fatalf("parseCorrectOptions() = %#v", options)
	}
}

func TestAppRetrievalAndCorrectionEndToEnd(t *testing.T) {
	t.Parallel()

	repositoryRoot := initializeTestRepository(t)
	config := testAppConfig(t)

	stop := startTestAppService(t, config)
	defer stop()

	runTestAppRecord(t, config, repositoryRoot, []string{"record", "commit", "Reusable integration behavior"})

	var initial QueryResult
	if err := NewClient(config.SocketPath).Call(context.Background(), "search", Query{Limit: 10}, &initial); err != nil {
		t.Fatalf("search recorded event: %v", err)
	}

	if len(initial.Events) != 1 {
		t.Fatalf("initial events = %#v", initial.Events)
	}

	event := initial.Events[0]
	showOutput := runTestAppCommand(t, config, []string{"show", "--id", string(event.ID), "--json"})

	var shown ShowResult
	decodeCLIResult(t, showOutput, &shown)
	assertShownEvent(t, shown, event.ID)

	searchOutput := runTestAppCommand(t, config, []string{"search", "--query", "integration", "--limit", "1", "--json"})

	var searched QueryResult
	decodeCLIResult(t, searchOutput, &searched)
	assertSearchedCount(t, searched, 1)

	correctOutput := runTestAppCommand(t, config, []string{
		"correct",
		"--target", string(event.ID),
		"--reason", "Clarify integration behavior",
		"--summary", "Corrected integration behavior",
		"--json",
	})

	var corrected CorrectResult
	decodeCLIResult(t, correctOutput, &corrected)
	assertCorrectedEvent(t, corrected, event.ID, "Corrected integration behavior")

	humanSearch := runTestAppCommand(t, config, []string{"search"})
	if !strings.Contains(humanSearch, string(corrected.Event.ID)) || !strings.Contains(humanSearch, "Corrected integration behavior") {
		t.Fatal("human search output does not contain the effective correction summary")
	}

	assertAppReportEndToEnd(t, config, corrected.Event)
	assertAppVerifyEndToEnd(t, config)
}

func assertAppReportEndToEnd(t *testing.T, config Config, event Event) {
	t.Helper()

	reportFrom := event.OccurredAt.Format(reportDateLayout)
	reportTo := event.OccurredAt.AddDate(0, 0, 1).Format(reportDateLayout)
	reportOutput := runTestAppCommand(t, config, []string{
		"report", "--from", reportFrom, "--to", reportTo, "--timezone", "UTC", "--json",
	})

	var report ReportResult
	decodeCLIResult(t, reportOutput, &report)
	assertReportCount(t, report, 1)

	humanReport := runTestAppCommand(t, config, []string{
		"report", "--from", reportFrom, "--to", reportTo, "--timezone", "UTC",
	})
	for _, expected := range []string{reportFrom, activityScopeName(event), string(event.ID), "total: 1 events, 0 minutes"} {
		if !strings.Contains(humanReport, expected) {
			t.Fatalf("human report = %q, want %q", humanReport, expected)
		}
	}
}

func assertAppVerifyEndToEnd(t *testing.T, config Config) {
	t.Helper()

	verifyOutput := runTestAppCommand(t, config, []string{"verify", "--full", "--json"})

	var result VerifyResult
	decodeCLIResult(t, verifyOutput, &result)

	if result.Scope != VerifyScopeFull || result.SegmentCount != 1 || result.EventCount != 2 {
		t.Fatalf("verify result = %#v", result)
	}

	humanOutput := runTestAppCommand(t, config, []string{"verify"})
	if !strings.Contains(humanOutput, "verified active journal") {
		t.Fatalf("human verify output = %q", humanOutput)
	}
}

func decodeCLIResult(t *testing.T, output string, target any) {
	t.Helper()

	if err := json.Unmarshal([]byte(output), target); err != nil {
		t.Fatalf("decode CLI output %q: %v", output, err)
	}
}

func assertShownEvent(t *testing.T, result ShowResult, wantID EventID) {
	t.Helper()

	if result.Event.ID != wantID {
		t.Fatalf("shown event = %#v", result.Event)
	}
}

func assertSearchedCount(t *testing.T, result QueryResult, want int) {
	t.Helper()

	if result.ReturnedCount != want {
		t.Fatalf("searched result = %#v", result)
	}
}

func assertCorrectedEvent(t *testing.T, result CorrectResult, targetID EventID, summary string) {
	t.Helper()

	if result.Event.Supersedes != targetID || result.Event.Summary != summary {
		t.Fatalf("corrected event = %#v", result.Event)
	}
}

func assertReportCount(t *testing.T, result ReportResult, want int) {
	t.Helper()

	if result.ReturnedCount != want {
		t.Fatalf("report result = %#v", result)
	}
}

func runTestAppCommand(t *testing.T, config Config, arguments []string) string {
	t.Helper()

	var output bytes.Buffer

	app := NewApp(config, &output, &bytes.Buffer{})

	if err := app.Run(context.Background(), arguments); err != nil {
		t.Fatalf("run %q: %v", arguments[0], err)
	}

	return output.String()
}

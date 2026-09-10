package lore

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParsePullRequestRecordWithCommonContext(t *testing.T) {
	t.Parallel()

	options, err := parseRecordOptions([]string{
		"pull-request",
		"--forge", "HTTPS://FORGE.EXAMPLE.TEST/",
		"--repository", "team/example",
		"--number", "42",
		"--action", "merged",
		"--url", "https://forge.example.test/team/example/pulls/42",
		"--at", "2026-09-03T10:15:00+02:00",
		"--reference-url", "https://grafana.example.test/alerting",
		"--reference-commit", "https://forge.example.test/team/example.git@0123456789abcdef0123456789abcdef01234567",
		"--reference-pull-request", "https://forge.example.test/team/example/pulls/41",
		"--source", "agent",
		"--harness", "codex",
		"--harness-version", "1.2.3",
		"--model", "openai/example-model",
		"Merged the typed activity change",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseRecordOptions() error = %v", err)
	}

	params := options.recordParams()
	if params.Type != ActivityTypePullRequest || params.Summary != "Merged the typed activity change" {
		t.Fatalf("record params = %#v", params)
	}

	wantAt := time.Date(2026, 9, 3, 8, 15, 0, 0, time.UTC)
	if !params.OccurredAt.Equal(wantAt) {
		t.Fatalf("occurred_at = %s, want %s", params.OccurredAt, wantAt)
	}

	wantScope := &ActivityScope{
		Type:     ScopeTypeRepository,
		Name:     "team/example",
		Identity: "https://forge.example.test/team/example",
	}
	if !reflect.DeepEqual(params.Scope, wantScope) {
		t.Fatalf("scope = %#v, want %#v", params.Scope, wantScope)
	}

	if len(params.References) != 3 || params.Provenance == nil || params.Provenance.Type != ProvenanceTypeAgent {
		t.Fatalf("common context = %#v, %#v", params.References, params.Provenance)
	}
}

func TestParseWorklogRecord(t *testing.T) {
	t.Parallel()

	worklog, err := parseRecordOptions([]string{
		"worklog",
		"--duration", "240",
		"--scope-type", "system",
		"--scope-name", "Production observability",
		"--scope-identity", "system:production-observability",
		"Configured Grafana alert routing",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse worklog: %v", err)
	}

	worklogParams := worklog.recordParams()
	if worklogParams.Type != ActivityTypeWorklog || worklogParams.Properties.DurationMinutes != 240 || worklogParams.Scope.Type != ScopeTypeSystem {
		t.Fatalf("worklog params = %#v", worklogParams)
	}
}

func TestParseNoteRecord(t *testing.T) {
	t.Parallel()

	note, err := parseRecordOptions([]string{
		"note",
		"--source", "human",
		"--reference-url", "https://example.test/decision",
		"Keep the deployment freeze until verification completes",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parse note: %v", err)
	}

	noteParams := note.recordParams()
	if noteParams.Type != ActivityTypeNote || noteParams.Properties != nil || noteParams.Scope != nil {
		t.Fatalf("note params = %#v", noteParams)
	}

	if noteParams.Provenance == nil || noteParams.Provenance.Type != ProvenanceTypeHuman {
		t.Fatalf("note provenance = %#v", noteParams.Provenance)
	}
}

func TestParseRecordOptionsRejectsInvalidTypedInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		arguments []string
	}{
		{name: "missing type"},
		{name: "unknown type", arguments: []string{"misc", "summary"}},
		{name: "commit occurrence", arguments: []string{"commit", "--at", "2026-09-03T08:00:00Z"}},
		{name: "commit scope", arguments: []string{"commit", "--scope-type", "project", "--scope-name", "Lore", "--scope-identity", "project:lore"}},
		{name: "pull request scope", arguments: []string{"pull-request", "--scope-type", "project", "--scope-name", "Lore", "--scope-identity", "project:lore", "summary"}},
		{name: "partial scope", arguments: []string{"note", "--scope-type", "project", "summary"}},
		{name: "invalid commit reference", arguments: []string{"note", "--reference-commit", "missing-separator", "summary"}},
		{name: "agent without harness", arguments: []string{"note", "--source", "agent", "summary"}},
		{name: "human with harness", arguments: []string{"note", "--source", "human", "--harness", "codex", "summary"}},
		{name: "pull request missing number", arguments: []string{"pull-request", "--forge", "https://forge.example.test", "--repository", "team/example", "--action", "opened", "summary"}},
		{name: "pull request invalid action", arguments: []string{"pull-request", "--forge", "https://forge.example.test", "--repository", "team/example", "--number", "1", "--action", "updated", "summary"}},
		{name: "worklog missing duration", arguments: []string{"worklog", "summary"}},
		{name: "note missing summary", arguments: []string{"note"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := parseRecordOptions(test.arguments, &bytes.Buffer{}); err == nil {
				t.Fatal("expected parse error")
			}
		})
	}
}

func TestAppRecordsEveryPeerTypeAndUsesNaturalIdempotency(t *testing.T) {
	t.Parallel()

	config := testAppConfig(t)

	stop := startTestAppService(t, config)
	defer stop()

	commands := [][]string{
		{"record", "pull-request", "--forge", "https://unreachable.invalid", "--repository", "team/example", "--number", "7", "--action", "opened", "Opened the delivery pull request"},
		{"record", "pull-request", "--forge", "https://unreachable.invalid", "--repository", "team/example", "--number", "7", "--action", "opened", "Opened the delivery pull request"},
		{"record", "pull-request", "--forge", "https://unreachable.invalid", "--repository", "team/example", "--number", "7", "--action", "merged", "Merged the delivery pull request"},
		{"record", "worklog", "--duration", "90", "Configured the deployment pipeline"},
		{"record", "worklog", "--duration", "90", "Configured the deployment pipeline"},
		{"record", "note", "--source", "human", "Preserve the rollback snapshot"},
		{"record", "note", "--source", "human", "Preserve the rollback snapshot"},
	}

	outputs := make([]string, 0, len(commands))
	for _, command := range commands {
		outputs = append(outputs, runTestAppCommand(t, config, command))
	}

	if !strings.HasPrefix(outputs[0], "recorded ") || !strings.HasPrefix(outputs[1], "already recorded ") {
		t.Fatalf("pull-request receipts = %q, %q", outputs[0], outputs[1])
	}

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("load recorded journal: %v", err)
	}

	if journal.Count() != 6 {
		t.Fatalf("journal count = %d, want 6", journal.Count())
	}

	result, err := journal.Search(context.Background(), Query{Limit: 10})
	if err != nil {
		t.Fatalf("search peer records: %v", err)
	}

	assertPeerTypeCounts(t, result.Events)
}

func assertPeerTypeCounts(t *testing.T, events []ActivityProjection) {
	t.Helper()

	counts := make(map[ActivityType]int)
	for _, event := range events {
		counts[event.Type]++
		if event.Type != ActivityTypeCommit && !event.OccurredAt.Equal(event.RecordedAt) {
			t.Fatalf("default occurrence time differs for %#v", event)
		}
	}

	want := map[ActivityType]int{
		ActivityTypePullRequest: 2,
		ActivityTypeWorklog:     2,
		ActivityTypeNote:        2,
	}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("type counts = %#v, want %#v", counts, want)
	}
}

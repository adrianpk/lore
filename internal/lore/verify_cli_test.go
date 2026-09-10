package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestParseVerifyOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		arguments []string
		want      verifyOptions
		wantErr   bool
	}{
		{name: "active"},
		{name: "full JSON", arguments: []string{"--full", "--json"}, want: verifyOptions{full: true, json: true}},
		{name: "scheduled", arguments: []string{"--scheduled"}, want: verifyOptions{scheduled: true}},
		{name: "conflicting scopes", arguments: []string{"--full", "--scheduled"}, wantErr: true},
		{name: "positional", arguments: []string{"extra"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseVerifyOptions(test.arguments, &bytes.Buffer{})
			if test.wantErr && err == nil {
				t.Fatal("expected parse error")
			}

			if !test.wantErr && (err != nil || got != test.want) {
				t.Fatalf("parseVerifyOptions() = %#v, %v", got, err)
			}
		})
	}
}

func TestAppScheduledVerificationPersistsMonthlyFullScope(t *testing.T) {
	t.Parallel()

	config := testAppConfig(t)

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("load journal: %v", err)
	}

	appendJournalEvent(t, journal, validMilestoneEvent())

	stop := startTestAppService(t, config)
	defer stop()

	app := NewApp(config, &bytes.Buffer{}, &bytes.Buffer{})
	app.now = func() time.Time {
		return time.Date(2026, 8, 17, 3, 0, 0, 0, time.Local)
	}

	first := runScheduledVerification(t, app)

	second := runScheduledVerification(t, app)
	if first.Scope != VerifyScopeFull || second.Scope != VerifyScopeActive {
		t.Fatalf("scheduled scopes = %q, %q", first.Scope, second.Scope)
	}

	stateContent, err := os.ReadFile(config.VerificationStatePath)
	if err != nil {
		t.Fatalf("read verification state: %v", err)
	}

	state, valid := decodeVerificationState(stateContent)
	if !valid || state.FullMonth != "2026-08" {
		t.Fatalf("verification state = %#v, %v", state, valid)
	}
}

func runScheduledVerification(t *testing.T, app *App) VerifyResult {
	t.Helper()

	var output bytes.Buffer

	app.stdout = &output

	if err := app.Run(context.Background(), []string{"verify", "--scheduled", "--json"}); err != nil {
		t.Fatalf("run scheduled verification: %v", err)
	}

	var result VerifyResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode scheduled result: %v", err)
	}

	return result
}

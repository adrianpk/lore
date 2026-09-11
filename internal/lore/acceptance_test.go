package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestKnowledgeWorkflowRoundTripsKindsThroughCLI(t *testing.T) {
	t.Parallel()

	config := acceptanceConfig(t)

	_, stop := startKnowledgeService(t, config)
	defer stop()

	stdout := &bytes.Buffer{}
	app := NewApp(config, stdout, &bytes.Buffer{})
	kinds := []EntryKind{EntryKindProcedure, EntryKindFact, EntryKindWarning, EntryKindReference}

	for _, kind := range kinds {
		stdout.Reset()

		err := app.Run(context.Background(), []string{
			"record", string(kind), "--title", "Acceptance " + string(kind),
			"--body", "Reusable knowledge for the acceptance workflow.",
			"--context", "project:lore", "--label", "acceptance", "--json",
		})
		if err != nil {
			t.Fatalf("record %s: %v", kind, err)
		}

		var result EntryMutationResult
		decodeAcceptanceJSON(t, stdout.Bytes(), &result)

		if result.Entry.Kind != kind || result.Entry.Contexts[0].Name != "lore" {
			t.Fatalf("record %s result = %#v", kind, result)
		}
	}

	stdout.Reset()

	if err := app.Run(context.Background(), []string{
		"search", "--context", "project:lore", "--label", "acceptance", "--json",
	}); err != nil {
		t.Fatalf("search: %v", err)
	}

	var search SearchEntriesResult
	decodeAcceptanceJSON(t, stdout.Bytes(), &search)
	assertAcceptanceKinds(t, search.Entries, kinds)
}

func TestKnowledgeWorkflowReviewPurgeSurvivesRestart(t *testing.T) {
	t.Parallel()

	config := acceptanceConfig(t)
	client, stop := startKnowledgeService(t, config)
	recorded := acceptanceRecord(t, client)
	revised := acceptanceRevise(t, client, recorded.Entry.ID)
	invalidated := callAcceptance[EntryMutationResult](t, client, "invalidate", LifecycleEntryParams{
		Target: revised.Entry.ID,
		Reason: "Acceptance lifecycle transition.",
	})

	review := callAcceptance[ReviewResult](t, client, "review", ReviewParams{
		Statuses: []EntryStatus{EntryStatusInvalidated},
	})
	if len(review.Candidates) != 1 || review.Candidates[0].Entry.ID != invalidated.Entry.ID {
		t.Fatalf("review result = %#v", review)
	}

	history := callAcceptance[ShowEntryResult](t, client, "show", ShowEntryParams{
		ID:      recorded.Entry.ID,
		History: true,
	})
	if len(history.History) != 3 {
		t.Fatalf("history revisions = %d, want 3", len(history.History))
	}

	dryRun := callAcceptance[PurgeResult](t, client, "purge", PurgeParams{
		IDs:    []EntryID{revised.Entry.ID},
		DryRun: true,
	})

	confirmed := callAcceptance[PurgeResult](t, client, "purge", PurgeParams{
		IDs:     []EntryID{revised.Entry.ID},
		Confirm: dryRun.ConfirmationToken,
	})
	if !confirmed.Purged || confirmed.RevisionCount != 3 {
		t.Fatalf("confirmed purge = %#v", confirmed)
	}

	stop()

	restarted, stopRestarted := startKnowledgeService(t, config)
	defer stopRestarted()

	verified := callAcceptance[VerifyEntriesResult](t, restarted, "verify", VerifyEntriesParams{})
	if verified.RevisionCount != 0 || verified.EffectiveCount != 0 {
		t.Fatalf("restarted verification = %#v", verified)
	}
}

func TestKnowledgeWorkflowSystemServiceAssets(t *testing.T) {
	t.Parallel()

	service := readRepositoryAsset(t, "lore.service")
	socket := readRepositoryAsset(t, "lore.socket")

	serviceParts := []string{
		"Description=Lore local knowledge service",
		"ExecStart=/usr/bin/env lore serve",
		"Requires=lore.socket",
		"UMask=0077",
	}
	socketParts := []string{
		"ListenStream=%t/lore/lore.sock",
		"SocketMode=0600",
		"DirectoryMode=0700",
		"Service=lore.service",
	}

	assertAssetParts(t, "lore.service", service, serviceParts)
	assertAssetParts(t, "lore.socket", socket, socketParts)
}

func acceptanceConfig(t *testing.T) Config {
	t.Helper()

	config := entryJournalConfig(t)
	config.RuntimeRoot = t.TempDir()
	config.SocketPath = config.RuntimeRoot + "/lore.sock"

	return config
}

func startKnowledgeService(t *testing.T, config Config) (*Client, func()) {
	t.Helper()

	journal := loadEntryJournal(t, config)

	listener, err := ListenUnix(config.SocketPath)
	if err != nil {
		t.Fatalf("listen service: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- NewServer(NewService(journal)).Serve(ctx, listener) }()

	var once sync.Once

	stop := func() {
		once.Do(func() {
			cancel()

			if serveErr := <-done; serveErr != nil {
				t.Errorf("serve knowledge workflow: %v", serveErr)
			}
		})
	}
	t.Cleanup(stop)

	return NewClient(config.SocketPath), stop
}

func acceptanceRecord(t *testing.T, client *Client) EntryMutationResult {
	t.Helper()

	return callAcceptance[EntryMutationResult](t, client, "record", RecordEntryParams{
		Kind:     EntryKindProcedure,
		Title:    "Recover acceptance service",
		Body:     "Inspect the service before one bounded restart.",
		Contexts: []Context{{Type: ContextTypeProject, Name: "lore"}},
		Labels:   []string{"acceptance"},
	})
}

func acceptanceRevise(t *testing.T, client *Client, target EntryID) EntryMutationResult {
	t.Helper()

	body := "Inspect logs, identify the failure, then restart once."

	return callAcceptance[EntryMutationResult](t, client, "revise", ReviseEntryParams{
		Target: target,
		Reason: "Clarify the bounded restart sequence.",
		Body:   &body,
	})
}

func callAcceptance[T any](t *testing.T, client *Client, operation string, params any) T {
	t.Helper()

	var result T
	if err := client.Call(context.Background(), operation, params, &result); err != nil {
		t.Fatalf("call %s: %v", operation, err)
	}

	return result
}

func decodeAcceptanceJSON(t *testing.T, encoded []byte, target any) {
	t.Helper()

	if err := json.Unmarshal(encoded, target); err != nil {
		t.Fatalf("decode acceptance JSON %q: %v", encoded, err)
	}
}

func assertAcceptanceKinds(t *testing.T, entries []Entry, want []EntryKind) {
	t.Helper()

	if len(entries) != len(want) {
		t.Fatalf("search entries = %d, want %d", len(entries), len(want))
	}

	found := make(map[EntryKind]bool, len(entries))
	for _, entry := range entries {
		found[entry.Kind] = true
	}

	for _, kind := range want {
		if !found[kind] {
			t.Errorf("search omitted kind %q", kind)
		}
	}
}

func assertAssetParts(t *testing.T, name, content string, parts []string) {
	t.Helper()

	for _, part := range parts {
		if !strings.Contains(content, part) {
			t.Errorf("%s omitted %q", name, part)
		}
	}
}

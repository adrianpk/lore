package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordEntryFlagsAndBodyFile(t *testing.T) {
	t.Parallel()

	bodyPath := filepath.Join(t.TempDir(), "knowledge.md")
	if err := os.WriteFile(bodyPath, []byte("1. Inspect the service.\n2. Verify the port."), 0o600); err != nil {
		t.Fatalf("write body fixture: %v", err)
	}

	options, err := parseRecordEntryOptions([]string{
		"procedure",
		"--title", "Recover Preview",
		"--body-file", bodyPath,
		"--context", "Project:Lore Project",
		"--label", "Service Recovery",
		"--reference", "file:docs/recovery.md",
		"--source", "agent",
		"--harness", "codex",
		"--json",
	}, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseRecordEntryOptions() error = %v", err)
	}

	if options.params.Body != "1. Inspect the service.\n2. Verify the port." {
		t.Fatalf("body = %q", options.params.Body)
	}

	if options.params.Contexts[0].Name != "lore-project" || options.params.Labels[0] != "Service Recovery" {
		t.Fatalf("parsed metadata = %#v", options.params)
	}

	if options.params.Provenance == nil || options.params.Provenance.Type != ProvenanceTypeAgent {
		t.Fatalf("provenance = %#v", options.params.Provenance)
	}
}

func TestRecordEntryJSONInput(t *testing.T) {
	t.Parallel()

	input := strings.NewReader("{\"title\":\"Inspect cache failures\",\"body\":\"Inspect logs.\\nConfirm the cache key.\",\"contexts\":[{\"type\":\"technology\",\"name\":\"Redis\"}],\"labels\":[\"Cache Failure\"]}")

	options, err := parseRecordEntryOptions(
		[]string{"warning", "--json-input", "--json"},
		input,
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("parseRecordEntryOptions() error = %v", err)
	}

	if options.params.Kind != EntryKindWarning || !options.jsonOutput {
		t.Fatalf("record options = %#v", options)
	}

	canonical, err := Entry{
		ID:         "01M26K00000000000000000000",
		Kind:       options.params.Kind,
		Status:     EntryStatusActive,
		Title:      options.params.Title,
		Body:       options.params.Body,
		RecordedAt: validKnowledgeEntry().RecordedAt,
		Contexts:   options.params.Contexts,
		Labels:     options.params.Labels,
	}.Canonicalize()
	if err != nil {
		t.Fatalf("Canonicalize() error = %v", err)
	}

	if canonical.Contexts[0].Name != "redis" || canonical.Labels[0] != "cache-failure" {
		t.Fatalf("canonical JSON metadata = %#v", canonical)
	}
}

func TestReviseEntryFlagsPreserveOmittedFields(t *testing.T) {
	t.Parallel()

	options, err := parseReviseEntryOptions([]string{
		"--target", "01M26K00000000000000000000",
		"--reason", "Clarify the title.",
		"--title", "Updated title",
		"--clear-labels",
	}, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseReviseEntryOptions() error = %v", err)
	}

	if options.params.Title == nil || *options.params.Title != "Updated title" {
		t.Fatalf("replacement title = %#v", options.params.Title)
	}

	if options.params.Body != nil || options.params.Contexts != nil {
		t.Fatalf("omitted replacements = %#v", options.params)
	}

	if options.params.Labels == nil || len(*options.params.Labels) != 0 {
		t.Fatalf("cleared labels = %#v", options.params.Labels)
	}
}

func TestSearchEntriesFlagsUseRepeatableFilters(t *testing.T) {
	t.Parallel()

	options, err := parseSearchEntriesOptions([]string{
		"--text", "recovery",
		"--kind", "procedure",
		"--kind", "warning",
		"--status", "active",
		"--context", "project:Lore",
		"--context", "host:Workstation",
		"--label", "Service Recovery",
		"--order", "ascending",
		"--limit", "5",
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseSearchEntriesOptions() error = %v", err)
	}

	if len(options.params.Kinds) != 2 || len(options.params.Contexts) != 2 {
		t.Fatalf("repeatable filters = %#v", options.params)
	}

	if options.params.Contexts[0].Name != "lore" || options.params.Labels[0] != "Service Recovery" {
		t.Fatalf("parsed filters = %#v", options.params)
	}
}

func TestRecordEntryInfersProjectContext(t *testing.T) {
	t.Parallel()

	root := initializeKnowledgeRepository(t, "Lore Project")
	app := NewApp(Config{}, &bytes.Buffer{}, &bytes.Buffer{})
	app.workingDirectory = func() (string, error) { return root, nil }

	contextValue, ok := app.inferProjectContext(context.Background())
	if !ok {
		t.Fatal("inferProjectContext() did not detect repository")
	}

	if contextValue.Name != "lore-project" {
		t.Fatalf("context name = %q", contextValue.Name)
	}

	if contextValue.Repository != "forge.example:2222/example/lore" {
		t.Fatalf("repository identity = %q", contextValue.Repository)
	}
}

func TestKnowledgeServiceWorkflow(t *testing.T) {
	t.Parallel()

	journal := loadEntryJournal(t, entryJournalConfig(t))
	service := NewService(journal)

	recorded := callKnowledgeService[EntryMutationResult](t, service, "record", RecordEntryParams{
		Kind:  EntryKindFact,
		Title: "Local socket ownership",
		Body:  "The Lore service is the single writer.",
	})

	archived := callKnowledgeService[EntryMutationResult](t, service, "archive", LifecycleEntryParams{
		Target: recorded.Entry.ID,
		Reason: "Temporarily hidden.",
	})

	shown := callKnowledgeService[ShowEntryResult](t, service, "show", ShowEntryParams{
		ID:      archived.Entry.ID,
		History: true,
	})
	if len(shown.History) != 2 {
		t.Fatalf("history = %#v", shown.History)
	}

	search := callKnowledgeService[SearchEntriesResult](t, service, "search", SearchEntriesParams{
		Statuses: []EntryStatus{EntryStatusArchived},
	})
	if len(search.Entries) != 1 || search.Entries[0].ID != archived.Entry.ID {
		t.Fatalf("search = %#v", search)
	}
}

func callKnowledgeService[T any](t *testing.T, service *Service, operation string, params any) T {
	t.Helper()

	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	response := service.Handle(context.Background(), Request{
		Protocol:  ProtocolVersion,
		Operation: operation,
		Params:    encoded,
	})
	if !response.OK {
		t.Fatalf("Handle(%q) response = %#v", operation, response)
	}

	var result T
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	return result
}

func initializeKnowledgeRepository(t *testing.T, directoryName string) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), directoryName)
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("create repository: %v", err)
	}

	runKnowledgeGit(t, root, "init", "-q")
	runKnowledgeGit(t, root, "remote", "add", "origin", "ssh://git@Forge.Example:2222/example/lore.git")

	return root
}

func runKnowledgeGit(t *testing.T, root string, arguments ...string) {
	t.Helper()

	commandArguments := append([]string{"-C", root}, arguments...)
	if output, err := exec.Command("git", commandArguments...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", arguments[0], err, output)
	}
}

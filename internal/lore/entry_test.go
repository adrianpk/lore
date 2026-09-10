package lore

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type entryValidationCase struct {
	name   string
	change func(*Entry)
}

func TestEntryValidateKinds(t *testing.T) {
	t.Parallel()

	// Every approved semantic kind uses the same canonical entry shape.
	kinds := []EntryKind{EntryKindProcedure, EntryKindFact, EntryKindWarning, EntryKindReference}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()

			entry := validKnowledgeEntry()
			entry.Kind = kind

			if err := entry.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestEntryValidateStatuses(t *testing.T) {
	t.Parallel()

	// Lifecycle state is independent from the entry's semantic kind.
	statuses := []EntryStatus{EntryStatusActive, EntryStatusArchived, EntryStatusInvalidated}
	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()

			entry := validKnowledgeEntry()
			entry.Status = status

			if err := entry.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestEntryCanonicalize(t *testing.T) {
	t.Parallel()

	// Canonicalization prepares flexible caller input without mutating it.
	entry := validKnowledgeEntry()
	entry.Title = "  Recover Preview Service  "
	entry.Body = "  Inspect the service before restarting it.\n  "
	entry.Contexts[0].Name = " Lore_Project "
	entry.Contexts[0].Repository = " forge.example/adrian/lore "
	entry.Labels = []string{" Service Recovery ", "CLI"}
	entry.References[0].Value = " docs/runbook.md "
	entry.References[0].Label = " Runbook "
	entry.Provenance.Harness = " codex "

	canonical, err := entry.Canonicalize()
	if err != nil {
		t.Fatalf("Canonicalize() error = %v", err)
	}

	assertCanonicalEntry(t, canonical)

	if entry.Contexts[0].Name != " Lore_Project " || entry.Provenance.Harness != " codex " {
		t.Fatal("Canonicalize() mutated caller-owned data")
	}
}

func TestEntryValidateIdentityAndContent(t *testing.T) {
	t.Parallel()

	nonUTC := time.Date(2026, 9, 10, 23, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	cases := []entryValidationCase{
		{name: "ID", change: func(entry *Entry) { entry.ID = "invalid" }},
		{name: "kind", change: func(entry *Entry) { entry.Kind = "memo" }},
		{name: "status", change: func(entry *Entry) { entry.Status = "deleted" }},
		{name: "empty title", change: func(entry *Entry) { entry.Title = "" }},
		{name: "title newline", change: func(entry *Entry) { entry.Title = "First\nSecond" }},
		{name: "title length", change: func(entry *Entry) { entry.Title = strings.Repeat("x", maxEntryTitleRunes+1) }},
		{name: "empty body", change: func(entry *Entry) { entry.Body = "" }},
		{name: "body whitespace", change: func(entry *Entry) { entry.Body = " body " }},
		{name: "body length", change: func(entry *Entry) { entry.Body = strings.Repeat("x", maxEntryBodyBytes+1) }},
		{name: "body control", change: func(entry *Entry) { entry.Body = "body\x00value" }},
		{name: "recorded time", change: func(entry *Entry) { entry.RecordedAt = time.Time{} }},
		{name: "recorded zone", change: func(entry *Entry) { entry.RecordedAt = nonUTC }},
		{name: "verified zone", change: func(entry *Entry) { entry.VerifiedAt = &nonUTC }},
		{name: "future verification", change: moveVerificationAfterRecording},
	}

	assertInvalidEntryCases(t, cases)
}

func TestEntryValidateCollections(t *testing.T) {
	t.Parallel()

	cases := []entryValidationCase{
		{name: "too many contexts", change: addTooManyContexts},
		{name: "duplicate context", change: duplicateContext},
		{name: "too many labels", change: addTooManyLabels},
		{name: "noncanonical label", change: func(entry *Entry) { entry.Labels[0] = "Service Recovery" }},
		{name: "duplicate label", change: func(entry *Entry) { entry.Labels = []string{"recovery", "recovery"} }},
		{name: "too many references", change: addTooManyReferences},
		{name: "duplicate reference", change: duplicateReference},
	}

	assertInvalidEntryCases(t, cases)
}

func TestContextValidate(t *testing.T) {
	t.Parallel()

	// All structured applicability types share one short canonical identity.
	types := []ContextType{
		ContextTypeProject,
		ContextTypeRepository,
		ContextTypeSystem,
		ContextTypeHost,
		ContextTypeTechnology,
		ContextTypeClient,
	}
	for _, contextType := range types {
		context := Context{Type: contextType, Name: "lore"}
		if err := context.Validate(); err != nil {
			t.Fatalf("Validate() type %q error = %v", contextType, err)
		}
	}

	invalid := []Context{
		{Type: "workspace", Name: "lore"},
		{Type: ContextTypeProject, Name: "Lore"},
		{Type: ContextTypeRepository, Name: "lore", Repository: "git@forge.example:adrian/lore"},
		{Type: ContextTypeRepository, Name: "lore", Repository: strings.Repeat("x", maxRepositoryBytes+1)},
	}
	for _, context := range invalid {
		if err := context.Validate(); err == nil {
			t.Fatalf("Validate() context %#v unexpectedly passed", context)
		}
	}
}

func TestNormalizeCanonicalName(t *testing.T) {
	t.Parallel()

	// Directory-style names and labels converge on the retrieval key grammar.
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "already canonical", value: "lore", want: "lore"},
		{name: "mixed separators", value: "  Lore_Project / API  ", want: "lore-project-api"},
		{name: "edge separators", value: "---Lore---", want: "lore"},
		{name: "empty", value: "___", wantErr: true},
		{name: "too long", value: strings.Repeat("x", maxCanonicalNameBytes+1), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := NormalizeCanonicalName(test.value)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}

				return
			}

			if err != nil {
				t.Fatalf("NormalizeCanonicalName() error = %v", err)
			}

			if got != test.want {
				t.Fatalf("NormalizeCanonicalName() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEntryReferenceValidate(t *testing.T) {
	t.Parallel()

	// Reference variants preserve opaque values while URLs and repositories
	// receive credential-specific validation.
	valid := []EntryReference{
		{Type: EntryReferenceTypeURL, Value: "https://example.test/runbook#restart"},
		{Type: EntryReferenceTypeFile, Value: "docs/@scope/runbook.md"},
		{Type: EntryReferenceTypeRepository, Value: "forge.example/adrian/lore"},
		{Type: EntryReferenceTypeCommit, Value: strings.Repeat("a", 40)},
	}
	for _, reference := range valid {
		if err := reference.Validate(); err != nil {
			t.Fatalf("Validate() reference %#v error = %v", reference, err)
		}
	}

	invalid := []EntryReference{
		{Type: "document", Value: "docs/runbook.md"},
		{Type: EntryReferenceTypeFile, Value: ""},
		{Type: EntryReferenceTypeURL, Value: "ftp://example.test/runbook"},
		{Type: EntryReferenceTypeURL, Value: "https://user:secret@example.test/runbook"},
		{Type: EntryReferenceTypeURL, Value: "https://example.test/runbook?token=secret"},
		{Type: EntryReferenceTypeRepository, Value: "git@forge.example:adrian/lore"},
		{Type: EntryReferenceTypeFile, Value: "docs/runbook.md", Label: "line\nbreak"},
	}
	for _, reference := range invalid {
		if err := reference.Validate(); err == nil {
			t.Fatalf("Validate() reference %#v unexpectedly passed", reference)
		}
	}
}

func TestProvenanceValidate(t *testing.T) {
	t.Parallel()

	// Provenance describes origin without assigning confidence or authority.
	types := []ProvenanceType{ProvenanceTypeHuman, ProvenanceTypeAgent, ProvenanceTypeIntegration}
	for _, provenanceType := range types {
		provenance := Provenance{Type: provenanceType, Harness: "codex", HarnessVersion: "1", Model: "gpt-5"}
		if err := provenance.Validate(); err != nil {
			t.Fatalf("Validate() type %q error = %v", provenanceType, err)
		}
	}

	invalid := []Provenance{
		{Type: "import"},
		{Type: ProvenanceTypeAgent, Harness: " codex"},
		{Type: ProvenanceTypeAgent, Model: strings.Repeat("x", maxProvenanceFieldRunes+1)},
	}
	for _, provenance := range invalid {
		if err := provenance.Validate(); err == nil {
			t.Fatalf("Validate() provenance %#v unexpectedly passed", provenance)
		}
	}
}

func TestNewEntryRevision(t *testing.T) {
	t.Parallel()

	// Revisions are complete entries that point to an earlier valid revision.
	previous := validKnowledgeEntry()
	replacement := previous
	replacement.Status = EntryStatusArchived
	replacement.VerifiedAt = nil

	recordedAt := previous.RecordedAt.Add(time.Hour)

	revision, err := NewEntryRevision(
		previous,
		replacement,
		"01M26K10000000000000000000",
		recordedAt,
		"Service retired.",
	)
	if err != nil {
		t.Fatalf("NewEntryRevision() error = %v", err)
	}

	if revision.Supersedes != previous.ID || revision.Status != EntryStatusArchived {
		t.Fatalf("NewEntryRevision() = %#v", revision)
	}

	if revision.RecordedAt != recordedAt || revision.RevisionReason != "Service retired." {
		t.Fatalf("NewEntryRevision() metadata = %#v", revision)
	}
}

func TestNewEntryRevisionRejectsBrokenContracts(t *testing.T) {
	t.Parallel()

	// Construction rejects local revision invariants before persistence checks
	// whether the predecessor is the effective chain head.
	previous := validKnowledgeEntry()
	recordedAt := previous.RecordedAt.Add(time.Hour)

	tests := []struct {
		name        string
		previous    Entry
		replacement Entry
		id          EntryID
		recordedAt  time.Time
		reason      string
	}{
		{name: "invalid predecessor", previous: Entry{}, replacement: previous, id: "01M26K10000000000000000000", recordedAt: recordedAt, reason: "Update."},
		{name: "kind change", previous: previous, replacement: entryWithKind(previous, EntryKindWarning), id: "01M26K10000000000000000000", recordedAt: recordedAt, reason: "Update."},
		{name: "self target", previous: previous, replacement: previous, id: previous.ID, recordedAt: recordedAt, reason: "Update."},
		{name: "missing reason", previous: previous, replacement: previous, id: "01M26K10000000000000000000", recordedAt: recordedAt},
		{name: "backdated", previous: previous, replacement: previous, id: "01M26K10000000000000000000", recordedAt: previous.RecordedAt.Add(-time.Second), reason: "Update."},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewEntryRevision(test.previous, test.replacement, test.id, test.recordedAt, test.reason)
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestEntryJSONContract(t *testing.T) {
	t.Parallel()

	// Root entries omit revision-only fields and retain stable public names.
	encoded, err := json.Marshal(validKnowledgeEntry())
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	required := []string{"id", "kind", "status", "title", "body", "recorded_at", "verified_at", "contexts", "labels", "references", "provenance"}
	for _, name := range required {
		if _, exists := object[name]; !exists {
			t.Errorf("JSON field %q is absent", name)
		}
	}

	for _, name := range []string{"supersedes", "revision_reason"} {
		if _, exists := object[name]; exists {
			t.Errorf("root JSON unexpectedly contains %q", name)
		}
	}
}

func validKnowledgeEntry() Entry {
	recordedAt := time.Date(2026, 9, 10, 21, 19, 39, 0, time.UTC)
	verifiedAt := recordedAt.Add(-time.Minute)

	return Entry{
		ID:         "01M26K00000000000000000000",
		Kind:       EntryKindProcedure,
		Status:     EntryStatusActive,
		Title:      "Recover preview service",
		Body:       "Inspect the service before restarting it.",
		RecordedAt: recordedAt,
		VerifiedAt: &verifiedAt,
		Contexts: []Context{
			{Type: ContextTypeProject, Name: "lore", Repository: "forge.example/adrian/lore"},
			{Type: ContextTypeHost, Name: "zeus"},
		},
		Labels: []string{"recovery", "service"},
		References: []EntryReference{
			{Type: EntryReferenceTypeFile, Value: "docs/runbook.md", Label: "runbook"},
		},
		Provenance: &Provenance{Type: ProvenanceTypeAgent, Harness: "codex", HarnessVersion: "1", Model: "gpt-5"},
	}
}

func assertCanonicalEntry(t *testing.T, entry Entry) {
	t.Helper()

	if entry.Title != "Recover Preview Service" || entry.Body != "Inspect the service before restarting it." {
		t.Fatalf("canonical content = %q / %q", entry.Title, entry.Body)
	}

	if entry.Contexts[0].Name != "lore-project" || entry.Contexts[0].Repository != "forge.example/adrian/lore" {
		t.Fatalf("canonical context = %#v", entry.Contexts[0])
	}

	if entry.Labels[0] != "service-recovery" || entry.Labels[1] != "cli" {
		t.Fatalf("canonical labels = %#v", entry.Labels)
	}

	if entry.References[0].Value != "docs/runbook.md" || entry.Provenance.Harness != "codex" {
		t.Fatalf("canonical metadata = %#v / %#v", entry.References[0], entry.Provenance)
	}
}

func assertInvalidEntryCases(t *testing.T, cases []entryValidationCase) {
	t.Helper()

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			entry := validKnowledgeEntry()
			test.change(&entry)

			if err := entry.Validate(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func moveVerificationAfterRecording(entry *Entry) {
	verifiedAt := entry.RecordedAt.Add(time.Second)
	entry.VerifiedAt = &verifiedAt
}

func addTooManyContexts(entry *Entry) {
	entry.Contexts = make([]Context, maxEntryContexts+1)
}

func duplicateContext(entry *Entry) {
	entry.Contexts = append(entry.Contexts, entry.Contexts[0])
}

func addTooManyLabels(entry *Entry) {
	entry.Labels = make([]string, maxEntryLabels+1)
}

func addTooManyReferences(entry *Entry) {
	entry.References = make([]EntryReference, maxEntryReferences+1)
}

func duplicateReference(entry *Entry) {
	entry.References = append(entry.References, entry.References[0])
}

func entryWithKind(entry Entry, kind EntryKind) Entry {
	entry.Kind = kind

	return entry
}

package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMigratorMapsLegacyHistoryAndIsDeterministic(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatalf("create source: %v", err)
	}

	events := representativeLegacyHistory(t)
	writeLegacyJournal(t, source, events...)

	sourceBefore := readJournalFiles(t, source)

	firstDestination := filepath.Join(root, "canonical-first")
	firstManifest := filepath.Join(root, "manifest-first.json")

	result, err := NewMigrator().Migrate(context.Background(), MigrationParams{
		Source: source, Destination: firstDestination, Manifest: firstManifest,
	})
	if err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	assertMigrationResult(t, result, events)
	assertMappedLegacyHistory(t, firstDestination, events)
	assertMigratedRetrieval(t, firstDestination, events)
	assertMigrationPermissions(t, firstDestination, firstManifest)
	assertPublishedManifest(t, firstManifest, result.Manifest)

	secondDestination := filepath.Join(root, "canonical-second")
	secondManifest := filepath.Join(root, "manifest-second.json")

	second, err := NewMigrator().Migrate(context.Background(), MigrationParams{
		Source: source, Destination: secondDestination, Manifest: secondManifest,
	})
	if err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	if !reflect.DeepEqual(result.Manifest, second.Manifest) {
		t.Fatalf("manifests differ:\nfirst=%#v\nsecond=%#v", result.Manifest, second.Manifest)
	}

	assertFileBytesEqual(t, firstManifest, secondManifest)
	assertJournalBytesEqual(t, firstDestination, secondDestination)

	if after := readJournalFiles(t, source); !reflect.DeepEqual(after, sourceBefore) {
		t.Fatal("migration modified the legacy source")
	}
}

func TestMigratorRejectsMalformedOrLossyLegacyHistory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		write func(*testing.T, string)
	}{
		{name: "unknown field", write: writeLegacyUnknownField},
		{name: "missing canonical summary", write: writeLossyLegacyEvent},
		{name: "changed correction snapshot", write: writeChangedLegacyCorrection},
		{name: "partial final line", write: writePartialLegacyLine},
		{name: "unexpected file", write: writeUnexpectedLegacyFile},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()

			source := filepath.Join(root, "source")
			if err := os.Mkdir(source, 0o700); err != nil {
				t.Fatalf("create source: %v", err)
			}

			test.write(t, source)

			params := MigrationParams{
				Source: source, Destination: filepath.Join(root, "destination"), Manifest: filepath.Join(root, "manifest.json"),
			}

			_, err := NewMigrator().Migrate(context.Background(), params)
			if !errors.Is(err, ErrMigrationInvalid) {
				t.Fatalf("Migrate() error = %v, want ErrMigrationInvalid", err)
			}

			assertPathMissing(t, params.Destination)
			assertPathMissing(t, params.Manifest)
		})
	}
}

func TestMigratorNeverOverwritesTargets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatalf("create source: %v", err)
	}

	writeLegacyJournal(t, source, representativeLegacyHistory(t)[0])

	for _, existing := range []string{"destination", "manifest"} {
		t.Run(existing, func(t *testing.T) {
			destination := filepath.Join(root, existing+"-destination")
			manifest := filepath.Join(root, existing+"-manifest.json")

			target := manifest
			if existing == "destination" {
				target = destination
			}

			if err := os.WriteFile(target, []byte("preserve\n"), 0o600); err != nil {
				t.Fatalf("create existing target: %v", err)
			}

			_, err := NewMigrator().Migrate(context.Background(), MigrationParams{
				Source: source, Destination: destination, Manifest: manifest,
			})
			if !errors.Is(err, ErrMigrationInvalid) {
				t.Fatalf("Migrate() error = %v, want ErrMigrationInvalid", err)
			}

			content, readErr := os.ReadFile(target)
			if readErr != nil || string(content) != "preserve\n" {
				t.Fatalf("existing target = %q, %v", content, readErr)
			}
		})
	}
}

func TestMigratorRollsBackDestinationWhenManifestPublicationFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatalf("create source: %v", err)
	}

	writeLegacyJournal(t, source, representativeLegacyHistory(t)[0])

	destination := filepath.Join(root, "destination")
	manifest := filepath.Join(root, "manifest.json")
	migrator := NewMigrator()
	migrator.files.link = func(_, newPath string) error {
		if newPath == manifest {
			return errors.New("injected manifest publication failure")
		}

		return fmt.Errorf("unexpected manifest target %q", newPath)
	}

	_, err := migrator.Migrate(context.Background(), MigrationParams{
		Source: source, Destination: destination, Manifest: manifest,
	})
	if !errors.Is(err, ErrMigrationPublication) {
		t.Fatalf("Migrate() error = %v, want ErrMigrationPublication", err)
	}

	assertPathMissing(t, destination)
	assertPathMissing(t, manifest)
}

func TestMigratorLeavesNoOutputOnDestinationPermissionFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatalf("create source: %v", err)
	}

	writeLegacyJournal(t, source, representativeLegacyHistory(t)[0])

	destination := filepath.Join(root, "destination")
	manifest := filepath.Join(root, "manifest.json")
	migrator := NewMigrator()
	migrator.files.mkdir = func(path string, _ os.FileMode) error {
		if path == destination {
			return os.ErrPermission
		}

		return fmt.Errorf("unexpected destination target %q", path)
	}

	_, err := migrator.Migrate(context.Background(), MigrationParams{
		Source: source, Destination: destination, Manifest: manifest,
	})
	if !errors.Is(err, ErrMigrationPublication) || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Migrate() error = %v, want publication permission error", err)
	}

	assertPathMissing(t, destination)
	assertPathMissing(t, manifest)
}

func representativeLegacyHistory(t *testing.T) []legacyActivity {
	t.Helper()

	first := validLegacyActivity(t, time.Date(2026, 8, 31, 23, 59, 0, 123000000, time.UTC), 51)
	correction := first
	correction.ID = nextEventID(t, time.Date(2026, 9, 1, 0, 1, 0, 456000000, time.UTC), 52)
	correction.RecordedAt = time.Date(2026, 9, 1, 0, 1, 0, 456000000, time.UTC)
	correction.Type = legacyEventTypeCorrection
	correction.Note = "Clarified reusable migration behavior"
	correction.TargetID = first.ID
	correction.Reason = "Clarify the preserved summary"

	local := validLegacyActivity(t, time.Date(2026, 9, 2, 10, 0, 0, 789000000, time.UTC), 53)
	local.ID = nextEventID(t, local.RecordedAt, 53)
	local.Repository = RepositoryReference{
		Name: "local-example", Root: "/workspace/local-example", Identity: "file:///workspace/local-example",
	}
	local.Git.Commit = "1123456789abcdef0123456789abcdef01234567"
	local.Note = ""

	return []legacyActivity{first, correction, local}
}

func validLegacyActivity(t *testing.T, recordedAt time.Time, marker byte) legacyActivity {
	t.Helper()

	return legacyActivity{
		SchemaVersion: legacySchemaVersion,
		ID:            nextEventID(t, recordedAt, marker),
		Type:          legacyEventTypeMilestone,
		RecordedAt:    recordedAt,
		Execution: &ExecutionContext{
			Harness: "codex", HarnessVersion: "1.0.0", Model: "openai/example",
		},
		Repository: RepositoryReference{
			Name:     "example",
			Root:     "/workspace/example",
			Identity: "ssh://git@example.test/team/example.git",
			Remote:   "ssh://git@example.test/team/example.git",
		},
		Git: GitReference{
			Commit:        "0123456789abcdef0123456789abcdef01234567",
			Branch:        "dev",
			Subject:       "feat: add reusable migration behavior",
			AuthoredAt:    recordedAt.Add(-10 * time.Minute),
			CommittedAt:   recordedAt.Add(-5 * time.Minute),
			WorktreeClean: true,
		},
		Note: "Reusable migration behavior",
	}
}

func writeLegacyJournal(t *testing.T, root string, events ...legacyActivity) {
	t.Helper()

	for _, event := range events {
		path := SegmentPath(root, event.RecordedAt)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatalf("create legacy segment directory: %v", err)
		}

		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal legacy event: %v", err)
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatalf("open legacy segment: %v", err)
		}

		_, writeErr := file.Write(append(encoded, '\n'))
		if err := errors.Join(writeErr, file.Close()); err != nil {
			t.Fatalf("write legacy event: %v", err)
		}
	}
}

func assertMigrationResult(t *testing.T, result MigrationResult, events []legacyActivity) {
	t.Helper()

	manifest := result.Manifest
	if manifest.Format != migrationManifestFormat || manifest.Source.EventCount != len(events) {
		t.Fatalf("migration manifest = %#v", manifest)
	}

	if manifest.Source.SegmentCount != 2 || manifest.Source.EffectiveEventCount != 2 {
		t.Fatalf("source summary = %#v", manifest.Source)
	}

	if !reflect.DeepEqual(manifest.Source.EventIDs, manifest.Destination.EventIDs) {
		t.Fatalf("event IDs differ: %#v", manifest)
	}

	if len(manifest.Source.CorrectionChains) != 2 || len(manifest.Source.CorrectionChains[0].EventIDs) != 2 {
		t.Fatalf("correction chains = %#v", manifest.Source.CorrectionChains)
	}

	for _, summary := range []MigrationJournalSummary{manifest.Source, manifest.Destination} {
		assertMigrationDigests(t, summary)
	}
}

const sha256HexLength = 64

func assertMigrationDigests(t *testing.T, summary MigrationJournalSummary) {
	t.Helper()

	if len(summary.SHA256) != sha256HexLength {
		t.Fatalf("journal SHA-256 = %q", summary.SHA256)
	}

	for _, segment := range summary.Segments {
		if len(segment.SHA256) != sha256HexLength {
			t.Fatalf("segment SHA-256 = %q", segment.SHA256)
		}
	}
}

func assertMappedLegacyHistory(t *testing.T, destination string, legacy []legacyActivity) {
	t.Helper()

	index, err := rebuildJournalIndex(context.Background(), destination)
	if err != nil {
		t.Fatalf("verify canonical destination: %v", err)
	}

	first := index.events[legacy[0].ID]
	correction := index.events[legacy[1].ID]
	local := index.events[legacy[2].ID]

	if first.Type != ActivityTypeCommit || first.Provenance.Type != ProvenanceTypeAgent {
		t.Fatalf("mapped milestone = %#v", first)
	}

	if correction.Supersedes != first.ID || correction.Summary != legacy[1].Note {
		t.Fatalf("mapped correction = %#v", correction)
	}

	if local.Summary != legacy[2].Git.Subject || local.Scope.Identity != legacy[2].Repository.Identity {
		t.Fatalf("mapped local repository = %#v", local)
	}

	for path, content := range readJournalFiles(t, destination) {
		assertCanonicalSegmentOmitsLegacyFields(t, path, content)
	}
}

func assertCanonicalSegmentOmitsLegacyFields(t *testing.T, path string, content []byte) {
	t.Helper()

	for _, forbidden := range []string{"schema_version", "\"root\"", "worktree_clean", "\"branch\"", "\"remote\""} {
		if strings.Contains(string(content), forbidden) {
			t.Fatalf("canonical segment %s contains %q: %s", path, forbidden, content)
		}
	}
}

func assertMigratedRetrieval(t *testing.T, destination string, legacy []legacyActivity) {
	t.Helper()

	journal := NewJournal(Config{
		FilesRoot:   destination,
		JournalRoot: destination,
		PendingPath: filepath.Join(destination, "pending.json"),
	})
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("load migrated journal: %v", err)
	}

	search, err := journal.Search(context.Background(), Query{Text: "clarified", Harness: "codex", Limit: 10})
	if err != nil {
		t.Fatalf("search migrated journal: %v", err)
	}

	if len(search.Events) != 1 || search.Events[0].ID != legacy[1].ID {
		t.Fatalf("migrated search = %#v", search)
	}

	report, err := journal.Report(context.Background(), ReportParams{
		From: "2026-08-31", To: "2026-09-03", Timezone: "UTC",
	})
	if err != nil {
		t.Fatalf("report migrated journal: %v", err)
	}

	if report.EventCount != 2 || report.WorklogMinutes != 0 {
		t.Fatalf("migrated report = %#v", report)
	}
}

func assertMigrationPermissions(t *testing.T, destination, manifest string) {
	t.Helper()

	assertPermission(t, destination, 0o700)
	assertPermission(t, filepath.Join(destination, "2026"), 0o700)
	assertPermission(t, filepath.Join(destination, "2026", "08.jsonl"), 0o600)
	assertPermission(t, manifest, 0o600)
}

func assertPublishedManifest(t *testing.T, path string, want MigrationManifest) {
	t.Helper()

	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read published manifest: %v", err)
	}

	var got MigrationManifest
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decode published manifest: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("published manifest = %#v, want %#v", got, want)
	}
}

func assertPermission(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}

	if info.Mode().Perm() != want {
		t.Fatalf("permissions for %s = %o, want %o", path, info.Mode().Perm(), want)
	}
}

func assertJournalBytesEqual(t *testing.T, first, second string) {
	t.Helper()

	firstFiles := readJournalFiles(t, first)

	secondFiles := readJournalFiles(t, second)
	if !reflect.DeepEqual(firstFiles, secondFiles) {
		t.Fatalf("journal bytes differ:\nfirst=%#v\nsecond=%#v", firstFiles, secondFiles)
	}
}

func readJournalFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()

	result := make(map[string][]byte)

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if entry.IsDir() {
			return nil
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		result[filepath.ToSlash(relative)] = content

		return nil
	})
	if err != nil {
		t.Fatalf("read journal tree: %v", err)
	}

	return result
}

func assertFileBytesEqual(t *testing.T, first, second string) {
	t.Helper()

	firstContent, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read %s: %v", first, err)
	}

	secondContent, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("read %s: %v", second, err)
	}

	if !bytes.Equal(firstContent, secondContent) {
		t.Fatalf("file bytes differ:\n%s\n%s", firstContent, secondContent)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("path %s exists or cannot be inspected: %v", path, err)
	}
}

func writeLegacyUnknownField(t *testing.T, root string) {
	t.Helper()

	event := representativeLegacyHistory(t)[0]

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal legacy event: %v", err)
	}

	raw := append(bytes.TrimSuffix(encoded, []byte{'}'}), []byte(",\"unknown\":true}\n")...)
	writeRawLegacySegment(t, root, event.RecordedAt, raw)
}

func writeLossyLegacyEvent(t *testing.T, root string) {
	t.Helper()

	event := representativeLegacyHistory(t)[0]
	event.Note = ""
	event.Git.Subject = ""
	writeLegacyJournal(t, root, event)
}

func writeChangedLegacyCorrection(t *testing.T, root string) {
	t.Helper()

	events := representativeLegacyHistory(t)[:2]
	events[1].Git.Branch = "changed"
	writeLegacyJournal(t, root, events...)
}

func writePartialLegacyLine(t *testing.T, root string) {
	t.Helper()

	event := representativeLegacyHistory(t)[0]

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal legacy event: %v", err)
	}

	writeRawLegacySegment(t, root, event.RecordedAt, encoded)
}

func writeUnexpectedLegacyFile(t *testing.T, root string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(root, "unexpected.txt"), []byte("unexpected\n"), 0o600); err != nil {
		t.Fatalf("write unexpected file: %v", err)
	}
}

func writeRawLegacySegment(t *testing.T, root string, recordedAt time.Time, content []byte) {
	t.Helper()

	path := SegmentPath(root, recordedAt)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create legacy segment directory: %v", err)
	}

	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write raw legacy segment: %v", err)
	}
}

func TestLegacyLocalRepositoryValidationMatchesOriginalContract(t *testing.T) {
	t.Parallel()

	reference := RepositoryReference{Name: "example", Root: "/workspace/example"}

	reference.Identity = (&url.URL{Scheme: "file", Path: filepath.ToSlash(reference.Root)}).String()
	if err := validateLegacyRepository(reference); err != nil {
		t.Fatalf("validateLegacyRepository() error = %v", err)
	}

	reference.Identity = fmt.Sprintf("git-root:%040d", 0)
	if err := validateLegacyRepository(reference); err == nil {
		t.Fatal("expected canonical local identity to be invalid for a legacy source")
	}
}

package lore

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	legacySchemaVersion     = "lore/v1"
	migrationManifestFormat = "lore/migration-manifest/v1"
)

var (
	// ErrMigrationInvalid identifies unsupported source data or unsafe paths.
	ErrMigrationInvalid = errors.New("migration is invalid")
	// ErrMigrationPublication identifies a failed destination publication.
	ErrMigrationPublication = errors.New("migration publication failed")
)

type legacyEventType string

const (
	legacyEventTypeMilestone  legacyEventType = "milestone"
	legacyEventTypeCorrection legacyEventType = "correction"
)

type legacyActivity struct {
	SchemaVersion string              `json:"schema_version"`
	ID            EventID             `json:"id"`
	Type          legacyEventType     `json:"type"`
	RecordedAt    time.Time           `json:"recorded_at"`
	Execution     *ExecutionContext   `json:"execution,omitempty"`
	Repository    RepositoryReference `json:"repository"`
	Git           GitReference        `json:"git"`
	Note          string              `json:"note,omitempty"`
	TargetID      EventID             `json:"target_id,omitempty"`
	Reason        string              `json:"reason,omitempty"`
}

// MigrationParams selects distinct offline source, destination, and manifest paths.
type MigrationParams struct {
	Source      string
	Destination string
	Manifest    string
}

// MigrationSegment records one relative segment digest.
type MigrationSegment struct {
	Path      string `json:"path"`
	LineCount int    `json:"line_count"`
	SHA256    string `json:"sha256"`
}

// MigrationCorrectionChain records one ordered supersession chain.
type MigrationCorrectionChain struct {
	RootID      EventID   `json:"root_id"`
	EffectiveID EventID   `json:"effective_id"`
	EventIDs    []EventID `json:"event_ids"`
}

// MigrationJournalSummary describes one complete source or destination journal.
type MigrationJournalSummary struct {
	SHA256              string                     `json:"sha256"`
	SegmentCount        int                        `json:"segment_count"`
	EventCount          int                        `json:"event_count"`
	EffectiveEventCount int                        `json:"effective_event_count"`
	EventIDs            []EventID                  `json:"event_ids"`
	CorrectionChains    []MigrationCorrectionChain `json:"correction_chains"`
	Segments            []MigrationSegment         `json:"segments"`
}

// MigrationManifest records deterministic cutover evidence.
type MigrationManifest struct {
	Format      string                  `json:"format"`
	Source      MigrationJournalSummary `json:"source"`
	Destination MigrationJournalSummary `json:"destination"`
}

// MigrationResult describes one published canonical journal.
type MigrationResult struct {
	Manifest MigrationManifest
}

type migrationPaths struct {
	source      string
	destination string
	manifest    string
}

type migrationFileOps struct {
	rename    func(string, string) error
	link      func(string, string) error
	mkdir     func(string, os.FileMode) error
	remove    func(string) error
	removeAll func(string) error
}

// Migrator transforms one frozen legacy journal without using the service.
type Migrator struct {
	files migrationFileOps
}

// NewMigrator constructs an offline migrator.
func NewMigrator() *Migrator {
	return &Migrator{files: migrationFileOps{
		rename: os.Rename, link: os.Link, mkdir: os.Mkdir, remove: os.Remove, removeAll: os.RemoveAll,
	}}
}

// Migrate verifies, transforms, verifies, and publishes one deterministic journal.
func (migrator *Migrator) Migrate(ctx context.Context, params MigrationParams) (result MigrationResult, resultErr error) {
	paths, err := normalizeMigrationPaths(params)
	if err != nil {
		return result, err
	}

	if err := validateMigrationTargets(paths); err != nil {
		return result, err
	}

	stagingRoot, err := createMigrationStagingRoot(paths.destination)
	if err != nil {
		return result, err
	}
	defer migrator.cleanupStagingRoot(stagingRoot, &result, &resultErr)

	manifest, err := buildMigrationManifest(ctx, paths, stagingRoot)
	if err != nil {
		return result, err
	}

	manifestTemporary, err := stageMigrationManifest(paths.manifest, manifest)
	if err != nil {
		return result, err
	}
	defer cleanupManifestTemporary(manifestTemporary, &result, &resultErr)

	if err := migrator.publish(paths, stagingRoot, manifestTemporary); err != nil {
		return result, err
	}

	return MigrationResult{Manifest: manifest}, nil
}

func (migrator *Migrator) cleanupStagingRoot(path string, result *MigrationResult, resultErr *error) {
	if err := migrator.files.removeAll(path); err != nil && *resultErr == nil {
		*result = MigrationResult{}
		*resultErr = fmt.Errorf("clean migration staging root: %w", err)
	}
}

func cleanupManifestTemporary(path string, result *MigrationResult, resultErr *error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) && *resultErr == nil {
		*result = MigrationResult{}
		*resultErr = fmt.Errorf("clean migration manifest: %w", err)
	}
}

func buildMigrationManifest(ctx context.Context, paths migrationPaths, stagingRoot string) (MigrationManifest, error) {
	sourceSummary, err := migrateLegacyJournal(ctx, paths.source, stagingRoot)
	if err != nil {
		return MigrationManifest{}, err
	}

	destinationSummary, err := summarizeCanonicalJournal(ctx, stagingRoot)
	if err != nil {
		return MigrationManifest{}, fmt.Errorf("%w: verify staged destination: %w", ErrMigrationInvalid, err)
	}

	if err := compareMigrationSemantics(sourceSummary, destinationSummary); err != nil {
		return MigrationManifest{}, err
	}

	currentSource, err := summarizeLegacyFiles(paths.source)
	if err != nil {
		return MigrationManifest{}, err
	}

	if currentSource.SHA256 != sourceSummary.SHA256 || !reflect.DeepEqual(currentSource.Segments, sourceSummary.Segments) {
		return MigrationManifest{}, fmt.Errorf("%w: source changed during migration", ErrMigrationInvalid)
	}

	return MigrationManifest{
		Format:      migrationManifestFormat,
		Source:      sourceSummary,
		Destination: destinationSummary,
	}, nil
}

func normalizeMigrationPaths(params MigrationParams) (migrationPaths, error) {
	source, err := resolveMigrationPath("source", params.Source)
	if err != nil {
		return migrationPaths{}, err
	}

	destination, err := resolveMigrationPath("destination", params.Destination)
	if err != nil {
		return migrationPaths{}, err
	}

	manifest, err := resolveMigrationPath("manifest", params.Manifest)
	if err != nil {
		return migrationPaths{}, err
	}

	paths := migrationPaths{source: source, destination: destination, manifest: manifest}

	return paths, validateMigrationPathRelations(paths)
}

func resolveMigrationPath(name, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%w: %s path is required", ErrMigrationInvalid, name)
	}

	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("%w: resolve %s path: %w", ErrMigrationInvalid, name, err)
	}

	return filepath.Clean(absolute), nil
}

func validateMigrationPathRelations(paths migrationPaths) error {
	if paths.source == paths.destination || paths.source == paths.manifest || paths.destination == paths.manifest {
		return fmt.Errorf("%w: source, destination, and manifest paths must be distinct", ErrMigrationInvalid)
	}

	if pathContains(paths.source, paths.destination) || pathContains(paths.destination, paths.source) {
		return fmt.Errorf("%w: source and destination must not contain each other", ErrMigrationInvalid)
	}

	if pathContains(paths.source, paths.manifest) || pathContains(paths.destination, paths.manifest) {
		return fmt.Errorf("%w: manifest must be outside source and destination", ErrMigrationInvalid)
	}

	return nil
}

func pathContains(parent, candidate string) bool {
	relative, err := filepath.Rel(parent, candidate)
	if err != nil {
		return false
	}

	return relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func validateMigrationTargets(paths migrationPaths) error {
	sourceInfo, err := os.Lstat(paths.source)
	if err != nil {
		return fmt.Errorf("%w: inspect source: %w", ErrMigrationInvalid, err)
	}

	if !sourceInfo.IsDir() || sourceInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: source must be a directory, not a symlink", ErrMigrationInvalid)
	}

	for _, target := range []struct {
		name string
		path string
	}{
		{name: "destination", path: paths.destination},
		{name: "manifest", path: paths.manifest},
	} {
		if _, err := os.Lstat(target.path); err == nil {
			return fmt.Errorf("%w: %s already exists", ErrMigrationInvalid, target.name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: inspect %s: %w", ErrMigrationInvalid, target.name, err)
		}

		parentInfo, err := os.Stat(filepath.Dir(target.path))
		if err != nil {
			return fmt.Errorf("%w: inspect %s parent: %w", ErrMigrationInvalid, target.name, err)
		}

		if !parentInfo.IsDir() {
			return fmt.Errorf("%w: %s parent is not a directory", ErrMigrationInvalid, target.name)
		}
	}

	return nil
}

func createMigrationStagingRoot(destination string) (string, error) {
	root, err := os.MkdirTemp(filepath.Dir(destination), ".lore-migration-*")
	if err != nil {
		return "", fmt.Errorf("create migration staging root: %w", err)
	}

	if err := os.Chmod(root, 0o700); err != nil {
		return "", errors.Join(fmt.Errorf("set migration staging permissions: %w", err), os.RemoveAll(root))
	}

	return root, nil
}

func (migrator *Migrator) publish(paths migrationPaths, stagingRoot, manifestTemporary string) error {
	if err := migrator.publishDestination(stagingRoot, paths.destination); err != nil {
		return err
	}

	rollback := func(operationErr error) error {
		cleanupErr := errors.Join(migrator.files.removeAll(paths.destination), migrator.removePublishedManifest(paths.manifest))

		return errors.Join(operationErr, cleanupErr)
	}

	if err := migrator.files.link(manifestTemporary, paths.manifest); err != nil {
		return rollback(fmt.Errorf("%w: publish manifest: %w", ErrMigrationPublication, err))
	}

	if err := syncDirectory(filepath.Dir(paths.manifest)); err != nil {
		return rollback(fmt.Errorf("%w: sync manifest parent: %w", ErrMigrationPublication, err))
	}

	if err := migrator.files.remove(manifestTemporary); err != nil {
		return rollback(fmt.Errorf("%w: remove staged manifest: %w", ErrMigrationPublication, err))
	}

	return nil
}

func (migrator *Migrator) removePublishedManifest(path string) error {
	err := migrator.files.remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func (migrator *Migrator) publishDestination(stagingRoot, destination string) error {
	if err := migrator.files.mkdir(destination, 0o700); err != nil {
		return fmt.Errorf("%w: reserve destination: %w", ErrMigrationPublication, err)
	}

	rollback := func(operationErr error) error {
		return errors.Join(operationErr, migrator.files.removeAll(destination))
	}

	entries, err := os.ReadDir(stagingRoot)
	if err != nil {
		return rollback(fmt.Errorf("%w: read staged destination: %w", ErrMigrationPublication, err))
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			return rollback(fmt.Errorf("%w: staged destination contains unexpected file", ErrMigrationPublication))
		}

		from := filepath.Join(stagingRoot, entry.Name())

		to := filepath.Join(destination, entry.Name())
		if err := migrator.files.rename(from, to); err != nil {
			return rollback(fmt.Errorf("%w: publish destination contents: %w", ErrMigrationPublication, err))
		}
	}

	if err := syncDirectory(destination); err != nil {
		return rollback(fmt.Errorf("%w: sync destination: %w", ErrMigrationPublication, err))
	}

	if err := syncDirectory(filepath.Dir(destination)); err != nil {
		return rollback(fmt.Errorf("%w: sync destination parent: %w", ErrMigrationPublication, err))
	}

	return nil
}

type legacyJournalIndex struct {
	events     map[EventID]legacyActivity
	milestones map[legacyMilestoneKey]EventID
	effective  map[legacyMilestoneKey]EventID
	roots      map[EventID]EventID
	chains     map[EventID][]EventID
	order      []EventID
}

type legacyMilestoneKey struct {
	repositoryIdentity string
	commit             string
}

func newLegacyJournalIndex() *legacyJournalIndex {
	return &legacyJournalIndex{
		events:     make(map[EventID]legacyActivity),
		milestones: make(map[legacyMilestoneKey]EventID),
		effective:  make(map[legacyMilestoneKey]EventID),
		roots:      make(map[EventID]EventID),
		chains:     make(map[EventID][]EventID),
		order:      make([]EventID, 0),
	}
}

func migrateLegacyJournal(ctx context.Context, sourceRoot, destinationRoot string) (MigrationJournalSummary, error) {
	paths, err := listLegacySegmentPaths(sourceRoot)
	if err != nil {
		return MigrationJournalSummary{}, err
	}

	legacyIndex := newLegacyJournalIndex()
	canonicalIndex := newJournalIndex()
	segments := make([]MigrationSegment, 0, len(paths))

	for _, sourcePath := range paths {
		if err := ctx.Err(); err != nil {
			return MigrationJournalSummary{}, err
		}

		segment, err := migrateLegacySegment(ctx, sourceRoot, destinationRoot, sourcePath, legacyIndex, canonicalIndex)
		if err != nil {
			return MigrationJournalSummary{}, err
		}

		segments = append(segments, segment)
	}

	if err := syncDirectory(destinationRoot); err != nil {
		return MigrationJournalSummary{}, fmt.Errorf("sync staged journal: %w", err)
	}

	return legacyMigrationSummary(legacyIndex, segments), nil
}

func listLegacySegmentPaths(root string) ([]string, error) {
	paths := make([]string, 0)

	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		return collectLegacySegmentPath(root, current, entry, walkErr, &paths)
	})
	if err != nil {
		return nil, fmt.Errorf("%w: list legacy journal: %w", ErrMigrationInvalid, err)
	}

	sort.Strings(paths)

	return paths, nil
}

func collectLegacySegmentPath(root, current string, entry fs.DirEntry, walkErr error, paths *[]string) error {
	if walkErr != nil {
		return walkErr
	}

	if current == root {
		return nil
	}

	relative, err := filepath.Rel(root, current)
	if err != nil {
		return err
	}

	if entry.IsDir() {
		if !validLegacyDirectory(relative) {
			return fmt.Errorf("unexpected directory %q", filepath.ToSlash(relative))
		}

		return nil
	}

	info, err := entry.Info()
	if err != nil {
		return err
	}

	if !info.Mode().IsRegular() || !validLegacySegmentPath(relative) {
		return fmt.Errorf("unexpected journal entry %q", filepath.ToSlash(relative))
	}

	*paths = append(*paths, current)

	return nil
}

func validLegacyDirectory(relative string) bool {
	return filepath.Dir(relative) == "." && len(relative) == 4 && decimalDigits(relative)
}

func validLegacySegmentPath(relative string) bool {
	year := filepath.Dir(relative)

	name := filepath.Base(relative)
	if !validLegacyDirectory(year) || len(name) != len("01.jsonl") || filepath.Ext(name) != ".jsonl" {
		return false
	}

	month, err := strconv.Atoi(strings.TrimSuffix(name, ".jsonl"))

	return err == nil && month >= 1 && month <= 12
}

func decimalDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}

	return true
}

func migrateLegacySegment(ctx context.Context, sourceRoot, destinationRoot, sourcePath string, legacyIndex *legacyJournalIndex, canonicalIndex *journalIndex) (MigrationSegment, error) {
	relative, err := filepath.Rel(sourceRoot, sourcePath)
	if err != nil {
		return MigrationSegment{}, fmt.Errorf("%w: resolve source segment: %w", ErrMigrationInvalid, err)
	}

	destinationPath := filepath.Join(destinationRoot, relative)
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o700); err != nil {
		return MigrationSegment{}, fmt.Errorf("create staged segment directory: %w", err)
	}

	if err := os.Chmod(filepath.Dir(destinationPath), 0o700); err != nil {
		return MigrationSegment{}, fmt.Errorf("set staged segment directory permissions: %w", err)
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return MigrationSegment{}, fmt.Errorf("%w: open legacy segment: %w", ErrMigrationInvalid, err)
	}

	destinationFile, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return MigrationSegment{}, errors.Join(fmt.Errorf("create staged segment: %w", err), sourceFile.Close())
	}

	segment, operationErr := migrateLegacySegmentContents(
		ctx,
		sourceRoot,
		destinationRoot,
		sourcePath,
		bufio.NewReader(sourceFile),
		destinationFile,
		legacyIndex,
		canonicalIndex,
	)
	if operationErr == nil {
		operationErr = destinationFile.Sync()
	}

	closeErr := errors.Join(destinationFile.Close(), sourceFile.Close())
	if err := errors.Join(operationErr, closeErr); err != nil {
		return MigrationSegment{}, err
	}

	if err := syncDirectory(filepath.Dir(destinationPath)); err != nil {
		return MigrationSegment{}, fmt.Errorf("sync staged segment directory: %w", err)
	}

	return segment, nil
}

func migrateLegacySegmentContents(ctx context.Context, sourceRoot, destinationRoot, sourcePath string, reader *bufio.Reader, writer io.Writer, legacyIndex *legacyJournalIndex, canonicalIndex *journalIndex) (MigrationSegment, error) {
	digest := sha256.New()
	lineNumber := 0

	for {
		if err := ctx.Err(); err != nil {
			return MigrationSegment{}, err
		}

		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			break
		}

		lineNumber++
		if errors.Is(readErr, io.EOF) {
			return MigrationSegment{}, legacyLineError(sourcePath, lineNumber, "line does not end with LF")
		}

		if readErr != nil {
			return MigrationSegment{}, fmt.Errorf("%w: read legacy segment: %w", ErrMigrationInvalid, readErr)
		}

		_, _ = digest.Write(line)
		if err := migrateLegacyLine(sourceRoot, destinationRoot, sourcePath, lineNumber, line, writer, legacyIndex, canonicalIndex); err != nil {
			return MigrationSegment{}, err
		}
	}

	relative, err := filepath.Rel(sourceRoot, sourcePath)
	if err != nil {
		return MigrationSegment{}, fmt.Errorf("%w: resolve migrated segment: %w", ErrMigrationInvalid, err)
	}

	return MigrationSegment{
		Path:      filepath.ToSlash(relative),
		LineCount: lineNumber,
		SHA256:    hex.EncodeToString(digest.Sum(nil)),
	}, nil
}

func migrateLegacyLine(sourceRoot, destinationRoot, sourcePath string, lineNumber int, framedLine []byte, writer io.Writer, legacyIndex *legacyJournalIndex, canonicalIndex *journalIndex) error {
	event, err := decodeLegacyLine(sourceRoot, sourcePath, lineNumber, framedLine)
	if err != nil {
		return err
	}

	if err := validateLegacyAgainstIndex(event, legacyIndex); err != nil {
		return legacyLineError(sourcePath, lineNumber, "%v", err)
	}

	relative, err := filepath.Rel(sourceRoot, sourcePath)
	if err != nil {
		return legacyLineError(sourcePath, lineNumber, "resolve source segment: %v", err)
	}

	mapped, err := mapAndValidateLegacyActivity(event, destinationRoot, relative, canonicalIndex)
	if err != nil {
		return legacyLineError(sourcePath, lineNumber, "%v", err)
	}

	canonicalLine, err := canonicalActivityLine(mapped)
	if err != nil {
		return legacyLineError(sourcePath, lineNumber, "%v", err)
	}

	if _, err := writer.Write(canonicalLine); err != nil {
		return fmt.Errorf("write staged segment: %w", err)
	}

	addLegacyToIndex(event, legacyIndex)
	addActivityToIndex(mapped, canonicalIndex)

	return nil
}

func decodeLegacyLine(sourceRoot, sourcePath string, lineNumber int, framedLine []byte) (legacyActivity, error) {
	line := bytes.TrimSuffix(framedLine, []byte{'\n'})
	if len(line) == 0 {
		return legacyActivity{}, legacyLineError(sourcePath, lineNumber, "blank line")
	}

	if len(line) > maxEventLineSize {
		return legacyActivity{}, legacyLineError(sourcePath, lineNumber, "event exceeds the size limit")
	}

	if !utf8.Valid(line) {
		return legacyActivity{}, legacyLineError(sourcePath, lineNumber, "event is not valid UTF-8")
	}

	event, err := decodeLegacyActivity(line)
	if err != nil {
		return legacyActivity{}, legacyLineError(sourcePath, lineNumber, "%v", err)
	}

	if SegmentPath(sourceRoot, event.RecordedAt) != sourcePath {
		return legacyActivity{}, legacyLineError(sourcePath, lineNumber, "event belongs to another segment")
	}

	return event, nil
}

func mapAndValidateLegacyActivity(event legacyActivity, destinationRoot, relativeSegment string, canonicalIndex *journalIndex) (Activity, error) {
	mapped := mapLegacyActivity(event)
	if SegmentPath(destinationRoot, mapped.RecordedAt) != filepath.Join(destinationRoot, relativeSegment) {
		return Activity{}, fmt.Errorf("mapped event belongs to another segment")
	}

	if err := mapped.Validate(); err != nil {
		return Activity{}, fmt.Errorf("map event: %w", err)
	}

	if err := validateActivityAgainstIndex(mapped, canonicalIndex); err != nil {
		return Activity{}, fmt.Errorf("map event: %w", err)
	}

	return mapped, nil
}

func decodeLegacyActivity(line []byte) (legacyActivity, error) {
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()

	var event legacyActivity
	if err := decoder.Decode(&event); err != nil {
		return legacyActivity{}, fmt.Errorf("decode legacy event: %w", err)
	}

	if err := requireJSONEnd(decoder); err != nil {
		return legacyActivity{}, fmt.Errorf("decode legacy event: %w", err)
	}

	if err := event.Validate(); err != nil {
		return legacyActivity{}, err
	}

	return event, nil
}

func (event legacyActivity) Validate() error {
	if err := event.validateCommon(); err != nil {
		return err
	}

	return event.validateType()
}

func (event legacyActivity) validateCommon() error {
	if event.SchemaVersion != legacySchemaVersion {
		return fmt.Errorf("validate legacy event: unsupported schema version %q", event.SchemaVersion)
	}

	if err := event.ID.Validate(); err != nil {
		return fmt.Errorf("validate legacy event ID: %w", err)
	}

	if err := validateUTCTime("recorded_at", event.RecordedAt); err != nil {
		return err
	}

	if event.Execution != nil {
		if err := event.Execution.Validate(); err != nil {
			return err
		}
	}

	if err := validateLegacyRepository(event.Repository); err != nil {
		return err
	}

	if err := event.Git.Validate(); err != nil {
		return err
	}

	if err := validateSignificanceText("note", event.Note, false); err != nil {
		return err
	}

	return nil
}

func (event legacyActivity) validateType() error {
	switch event.Type {
	case legacyEventTypeMilestone:
		if event.TargetID != "" || event.Reason != "" {
			return fmt.Errorf("validate legacy milestone: correction fields must be empty")
		}
	case legacyEventTypeCorrection:
		if err := event.TargetID.Validate(); err != nil {
			return fmt.Errorf("validate legacy correction target: %w", err)
		}

		if err := validateSignificanceText("reason", event.Reason, true); err != nil {
			return err
		}
	default:
		return fmt.Errorf("validate legacy event: unsupported type %q", event.Type)
	}

	return nil
}

func validateLegacyRepository(reference RepositoryReference) error {
	if err := validateLegacyRepositoryCommon(reference); err != nil {
		return err
	}

	if reference.Remote == "" {
		return validateLegacyLocalRepository(reference)
	}

	return validateLegacyRemoteRepository(reference)
}

func validateLegacyRepositoryCommon(reference RepositoryReference) error {
	if err := validateIdentifier("repository name", reference.Name, true); err != nil {
		return err
	}

	if !filepath.IsAbs(reference.Root) || filepath.Clean(reference.Root) != reference.Root {
		return fmt.Errorf("validate legacy repository: root must be a clean absolute path")
	}

	if reference.Name != filepath.Base(reference.Root) {
		return fmt.Errorf("validate legacy repository: name does not match root")
	}

	if err := validateIdentifier("repository identity", reference.Identity, true); err != nil {
		return err
	}

	if err := validateIdentifier("repository remote", reference.Remote, false); err != nil {
		return err
	}

	return nil
}

func validateLegacyLocalRepository(reference RepositoryReference) error {
	expected := (&url.URL{Scheme: "file", Path: filepath.ToSlash(reference.Root)}).String()
	if reference.Identity != expected {
		return fmt.Errorf("validate legacy repository: local identity does not match root")
	}

	return nil
}

func validateLegacyRemoteRepository(reference RepositoryReference) error {
	normalized, err := NormalizeRepositoryRemote(reference.Remote)
	if err != nil {
		return fmt.Errorf("validate legacy repository remote: %w", err)
	}

	if reference.Remote != normalized || reference.Identity != normalized {
		return fmt.Errorf("validate legacy repository: remote identity is not canonical")
	}

	return nil
}

func validateLegacyAgainstIndex(event legacyActivity, index *legacyJournalIndex) error {
	if _, ok := index.events[event.ID]; ok {
		return fmt.Errorf("duplicate event ID %q", event.ID)
	}

	key := legacyActivityKey(event)
	switch event.Type {
	case legacyEventTypeMilestone:
		if _, ok := index.milestones[key]; ok {
			return fmt.Errorf("duplicate legacy milestone identity")
		}
	case legacyEventTypeCorrection:
		target, ok := index.events[event.TargetID]
		if !ok {
			return fmt.Errorf("legacy correction target %q does not exist", event.TargetID)
		}

		if legacyActivityKey(target) != key {
			return fmt.Errorf("legacy correction identity does not match target %q", event.TargetID)
		}

		if index.effective[key] != event.TargetID {
			return fmt.Errorf("legacy correction target %q is stale", event.TargetID)
		}

		if !sameLegacySnapshot(event, target) {
			return fmt.Errorf("legacy correction snapshot does not match target %q", event.TargetID)
		}
	}

	return nil
}

func legacyActivityKey(event legacyActivity) legacyMilestoneKey {
	return legacyMilestoneKey{repositoryIdentity: event.Repository.Identity, commit: event.Git.Commit}
}

func sameLegacySnapshot(left, right legacyActivity) bool {
	return left.Repository == right.Repository && left.Git == right.Git && reflect.DeepEqual(left.Execution, right.Execution)
}

func addLegacyToIndex(event legacyActivity, index *legacyJournalIndex) {
	key := legacyActivityKey(event)

	root := event.ID
	if event.Type == legacyEventTypeCorrection {
		root = index.roots[event.TargetID]
	} else {
		index.milestones[key] = event.ID
	}

	index.events[event.ID] = event
	index.effective[key] = event.ID
	index.roots[event.ID] = root
	index.chains[root] = append(index.chains[root], event.ID)
	index.order = append(index.order, event.ID)
}

func mapLegacyActivity(event legacyActivity) Activity {
	summary := event.Note
	if summary == "" {
		summary = event.Git.Subject
	}

	properties := &ActivityProperties{ObjectID: event.Git.Commit, Subject: event.Git.Subject}
	if !event.Git.AuthoredAt.Equal(event.Git.CommittedAt) {
		authoredAt := event.Git.AuthoredAt
		properties.AuthoredAt = &authoredAt
	}

	mapped := Activity{
		ID:         event.ID,
		Type:       ActivityTypeCommit,
		OccurredAt: event.Git.CommittedAt,
		RecordedAt: event.RecordedAt,
		Summary:    summary,
		Scope: &ActivityScope{
			Type:     ScopeTypeRepository,
			Name:     event.Repository.Name,
			Identity: event.Repository.Identity,
		},
		Properties: properties,
	}

	if event.Execution != nil {
		mapped.Provenance = &ActivityProvenance{
			Type:           ProvenanceTypeAgent,
			Harness:        event.Execution.Harness,
			HarnessVersion: event.Execution.HarnessVersion,
			Model:          event.Execution.Model,
		}
	}

	if event.Type == legacyEventTypeCorrection {
		mapped.Supersedes = event.TargetID
		mapped.CorrectionReason = event.Reason
	}

	return mapped
}

func legacyMigrationSummary(index *legacyJournalIndex, segments []MigrationSegment) MigrationJournalSummary {
	return MigrationJournalSummary{
		SHA256:              journalDigest(segments),
		SegmentCount:        len(segments),
		EventCount:          len(index.order),
		EffectiveEventCount: len(index.chains),
		EventIDs:            append([]EventID(nil), index.order...),
		CorrectionChains:    legacyCorrectionChains(index),
		Segments:            segments,
	}
}

func legacyCorrectionChains(index *legacyJournalIndex) []MigrationCorrectionChain {
	return orderedCorrectionChains(index.order, index.roots, index.chains)
}

func orderedCorrectionChains(order []EventID, roots map[EventID]EventID, chains map[EventID][]EventID) []MigrationCorrectionChain {
	result := make([]MigrationCorrectionChain, 0, len(chains))
	seen := make(map[EventID]struct{})

	for _, id := range order {
		root := roots[id]
		if _, ok := seen[root]; ok {
			continue
		}

		seen[root] = struct{}{}

		eventIDs := append([]EventID(nil), chains[root]...)
		result = append(result, MigrationCorrectionChain{
			RootID:      root,
			EffectiveID: eventIDs[len(eventIDs)-1],
			EventIDs:    eventIDs,
		})
	}

	return result
}

func summarizeCanonicalJournal(ctx context.Context, root string) (MigrationJournalSummary, error) {
	index, err := rebuildJournalIndex(ctx, root)
	if err != nil {
		return MigrationJournalSummary{}, err
	}

	fileSummary, err := summarizeJournalFiles(root, false)
	if err != nil {
		return MigrationJournalSummary{}, err
	}

	chains := make(map[EventID][]EventID)

	for _, id := range index.order {
		rootID := index.roots[id]
		chains[rootID] = append(chains[rootID], id)
	}

	fileSummary.EventCount = len(index.order)
	fileSummary.EffectiveEventCount = len(chains)

	fileSummary.EventIDs = append([]EventID(nil), index.order...)
	fileSummary.CorrectionChains = orderedCorrectionChains(index.order, index.roots, chains)

	return fileSummary, nil
}

func summarizeLegacyFiles(root string) (MigrationJournalSummary, error) {
	return summarizeJournalFiles(root, true)
}

func summarizeJournalFiles(root string, legacy bool) (MigrationJournalSummary, error) {
	var (
		paths []string
		err   error
	)
	if legacy {
		paths, err = listLegacySegmentPaths(root)
	} else {
		paths, err = listSegmentPaths(root)
	}

	if err != nil {
		return MigrationJournalSummary{}, err
	}

	segments := make([]MigrationSegment, 0, len(paths))
	for _, path := range paths {
		digest, lineCount, err := digestFile(path)
		if err != nil {
			return MigrationJournalSummary{}, err
		}

		relative, err := filepath.Rel(root, path)
		if err != nil {
			return MigrationJournalSummary{}, fmt.Errorf("resolve journal segment: %w", err)
		}

		segments = append(segments, MigrationSegment{
			Path:      filepath.ToSlash(relative),
			LineCount: lineCount,
			SHA256:    digest,
		})
	}

	return MigrationJournalSummary{
		SHA256:       journalDigest(segments),
		SegmentCount: len(segments),
		Segments:     segments,
	}, nil
}

func digestFile(path string) (digestString string, lineCount int, resultErr error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("read journal digest %q: %w", path, err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, file.Close())
	}()

	digest := sha256.New()
	buffer := make([]byte, 64<<10)
	last := byte(0)
	total := 0

	for {
		read, readErr := file.Read(buffer)
		if read > 0 {
			chunk := buffer[:read]
			_, _ = digest.Write(chunk)
			lineCount += bytes.Count(chunk, []byte{'\n'})
			last = chunk[len(chunk)-1]
			total += read
		}

		if errors.Is(readErr, io.EOF) {
			break
		}

		if readErr != nil {
			return "", 0, fmt.Errorf("read journal digest %q: %w", path, readErr)
		}
	}

	if total > 0 && last != '\n' {
		return "", 0, fmt.Errorf("%w: %s does not end with LF", ErrMigrationInvalid, path)
	}

	return hex.EncodeToString(digest.Sum(nil)), lineCount, nil
}

func journalDigest(segments []MigrationSegment) string {
	var input strings.Builder

	for _, segment := range segments {
		input.WriteString(segment.Path)
		input.WriteByte(0)
		input.WriteString(strconv.Itoa(segment.LineCount))
		input.WriteByte(0)
		input.WriteString(segment.SHA256)
		input.WriteByte(0)
	}

	digest := sha256.Sum256([]byte(input.String()))

	return hex.EncodeToString(digest[:])
}

func compareMigrationSemantics(source, destination MigrationJournalSummary) error {
	if source.SegmentCount != destination.SegmentCount || source.EventCount != destination.EventCount {
		return fmt.Errorf("%w: staged destination counts do not match source", ErrMigrationInvalid)
	}

	sameIdentity := source.EffectiveEventCount == destination.EffectiveEventCount &&
		reflect.DeepEqual(source.EventIDs, destination.EventIDs) &&
		reflect.DeepEqual(source.CorrectionChains, destination.CorrectionChains)
	if !sameIdentity {
		return fmt.Errorf("%w: staged destination identity does not match source", ErrMigrationInvalid)
	}

	for index := range source.Segments {
		sameSegment := source.Segments[index].Path == destination.Segments[index].Path &&
			source.Segments[index].LineCount == destination.Segments[index].LineCount
		if !sameSegment {
			return fmt.Errorf("%w: staged destination segments do not match source", ErrMigrationInvalid)
		}
	}

	return nil
}

func stageMigrationManifest(path string, manifest MigrationManifest) (string, error) {
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode migration manifest: %w", err)
	}

	encoded = append(encoded, '\n')

	file, err := os.CreateTemp(filepath.Dir(path), ".lore-manifest-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create migration manifest: %w", err)
	}

	temporary := file.Name()

	if err := file.Chmod(0o600); err != nil {
		return "", errors.Join(fmt.Errorf("set migration manifest permissions: %w", err), file.Close(), os.Remove(temporary))
	}

	if _, err := file.Write(encoded); err != nil {
		return "", errors.Join(fmt.Errorf("write migration manifest: %w", err), file.Close(), os.Remove(temporary))
	}

	if err := file.Sync(); err != nil {
		return "", errors.Join(fmt.Errorf("sync migration manifest: %w", err), file.Close(), os.Remove(temporary))
	}

	if err := file.Close(); err != nil {
		return "", errors.Join(fmt.Errorf("close migration manifest: %w", err), os.Remove(temporary))
	}

	return temporary, nil
}

func legacyLineError(path string, line int, format string, arguments ...any) error {
	return fmt.Errorf("%w: %s:%d: %s", ErrMigrationInvalid, path, line, fmt.Sprintf(format, arguments...))
}

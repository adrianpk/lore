package lore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const entryPendingSchemaVersion = "lore/entry-pending/v1"

type entryPendingOperation struct {
	SchemaVersion string  `json:"schema_version"`
	EntryID       EntryID `json:"entry_id"`
	Segment       string  `json:"segment"`
	OriginalSize  int64   `json:"original_size"`
	Append        string  `json:"append"`
}

func publishEntry(config Config, segmentPath string, id EntryID, line []byte) error {
	operation, err := prepareEntryPendingOperation(config.EntriesRoot, segmentPath, id, line)
	if err != nil {
		return err
	}

	pendingPath := filepath.Join(config.EntryPendingRoot, string(id)+".json")
	if err := writeEntryPendingOperation(config.EntryPendingRoot, pendingPath, operation); err != nil {
		return err
	}

	_, err = recoverEntryPending(config)

	return err
}

func prepareEntryPendingOperation(root, segmentPath string, id EntryID, line []byte) (entryPendingOperation, error) {
	relative, err := filepath.Rel(root, segmentPath)
	if err != nil {
		return entryPendingOperation{}, fmt.Errorf("prepare entry pending operation: %w", err)
	}

	if !validRelativeSegment(relative) {
		return entryPendingOperation{}, fmt.Errorf("prepare entry pending operation: segment is outside the entries root")
	}

	originalSize, err := segmentSize(segmentPath)
	if err != nil {
		return entryPendingOperation{}, err
	}

	separator, err := entryAppendSeparator(segmentPath, originalSize)
	if err != nil {
		return entryPendingOperation{}, err
	}

	appendBytes := append(separator, line...)
	operation := entryPendingOperation{
		SchemaVersion: entryPendingSchemaVersion,
		EntryID:       id,
		Segment:       filepath.ToSlash(relative),
		OriginalSize:  originalSize,
		Append:        string(appendBytes),
	}

	if _, _, err := validateEntryPendingOperation(root, operation); err != nil {
		return entryPendingOperation{}, err
	}

	return operation, nil
}

func recoverEntryPending(config Config) (bool, error) {
	pendingPath, exists, err := findEntryPendingPath(config.EntryPendingRoot)
	if err != nil || !exists {
		return false, err
	}

	operation, err := readEntryPendingOperation(pendingPath)
	if err != nil {
		return false, err
	}

	segmentPath, appendBytes, err := validateEntryPendingOperation(config.EntriesRoot, operation)
	if err != nil {
		return false, err
	}

	wantPendingPath := filepath.Join(config.EntryPendingRoot, string(operation.EntryID)+".json")
	if pendingPath != wantPendingPath {
		return false, corruptEntryPending(pendingPath, operation.OriginalSize, "filename does not match entry ID")
	}

	if err := recoverEntryPendingOperation(pendingPath, segmentPath, operation.OriginalSize, appendBytes); err != nil {
		return false, err
	}

	return true, nil
}

func recoverEntryPendingOperation(pendingPath, segmentPath string, originalSize int64, appendBytes []byte) error {
	currentSize, err := segmentSize(segmentPath)
	if err != nil {
		return err
	}

	if currentSize < originalSize {
		return corruptEntryPending(segmentPath, currentSize, "segment is shorter than its recorded original size")
	}

	appendedSize := currentSize - originalSize
	if appendedSize > int64(len(appendBytes)) {
		return corruptEntryPending(segmentPath, originalSize, "segment contains bytes beyond the pending entry")
	}

	if appendedSize == 0 {
		return appendAndClearEntryPending(pendingPath, segmentPath, appendBytes)
	}

	suffix, err := readSegmentRange(segmentPath, originalSize, appendedSize)
	if err != nil {
		return err
	}

	if bytes.Equal(suffix, appendBytes) {
		return clearEntryPendingOperation(pendingPath)
	}

	if bytes.Equal(suffix, appendBytes[:len(suffix)]) {
		if err := truncateSegment(segmentPath, originalSize); err != nil {
			return err
		}

		return appendAndClearEntryPending(pendingPath, segmentPath, appendBytes)
	}

	return corruptEntryPending(segmentPath, originalSize, "terminal bytes do not match the pending entry")
}

func appendAndClearEntryPending(pendingPath, segmentPath string, appendBytes []byte) error {
	if err := appendEventLine(segmentPath, appendBytes); err != nil {
		return err
	}

	return clearEntryPendingOperation(pendingPath)
}

func validateEntryPendingOperation(root string, operation entryPendingOperation) (string, []byte, error) {
	relative, appendBytes, err := validateEntryPendingMetadata(operation)
	if err != nil {
		return "", nil, err
	}

	entryLine := bytes.TrimPrefix(appendBytes, []byte{'\n'})

	entry, err := decodeCanonicalEntry(entryLine[:len(entryLine)-1])
	if err != nil {
		return "", nil, corruptEntryPending(operation.Segment, operation.OriginalSize, "decode pending entry: %v", err)
	}

	if entry.ID != operation.EntryID {
		return "", nil, corruptEntryPending(operation.Segment, operation.OriginalSize, "entry ID does not match metadata")
	}

	canonical, err := canonicalEntryLine(entry)
	if err != nil {
		return "", nil, fmt.Errorf("encode pending entry: %w", err)
	}

	if !bytes.Equal(entryLine, canonical) {
		return "", nil, corruptEntryPending(operation.Segment, operation.OriginalSize, "pending entry is not canonical JSON")
	}

	segmentPath := filepath.Join(root, relative)
	if EntrySegmentPath(root, entry.RecordedAt) != segmentPath {
		return "", nil, corruptEntryPending(operation.Segment, operation.OriginalSize, "pending entry belongs to another segment")
	}

	return segmentPath, appendBytes, nil
}

func validateEntryPendingMetadata(operation entryPendingOperation) (string, []byte, error) {
	if operation.SchemaVersion != entryPendingSchemaVersion {
		return "", nil, corruptEntryPending(operation.Segment, 0, "unsupported pending schema version")
	}

	if err := operation.EntryID.Validate(); err != nil {
		return "", nil, corruptEntryPending(operation.Segment, 0, "%v", err)
	}

	relative := filepath.FromSlash(operation.Segment)
	if !validRelativeSegment(relative) {
		return "", nil, corruptEntryPending(operation.Segment, 0, "invalid pending segment path")
	}

	if operation.OriginalSize < 0 {
		return "", nil, corruptEntryPending(operation.Segment, 0, "negative original segment size")
	}

	appendBytes := []byte(operation.Append)
	if err := validateEntryPendingAppend(operation, appendBytes); err != nil {
		return "", nil, err
	}

	return relative, appendBytes, nil
}

func validateEntryPendingAppend(operation entryPendingOperation, appendBytes []byte) error {
	if len(appendBytes) == 0 || len(appendBytes) > maxEventLineSize+1 || appendBytes[len(appendBytes)-1] != '\n' {
		return corruptEntryPending(operation.Segment, operation.OriginalSize, "invalid pending entry framing")
	}

	lineFeeds := bytes.Count(appendBytes, []byte{'\n'})
	if lineFeeds == 1 {
		return nil
	}

	if lineFeeds == 2 && appendBytes[0] == '\n' {
		return nil
	}

	return corruptEntryPending(operation.Segment, operation.OriginalSize, "pending entry contains an embedded newline")
}

func writeEntryPendingOperation(root, pendingPath string, operation entryPendingOperation) error {
	if _, exists, err := findEntryPendingPath(root); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("write entry pending operation: another operation already exists")
	}

	encoded, err := json.Marshal(operation)
	if err != nil {
		return fmt.Errorf("encode entry pending operation: %w", err)
	}

	encoded = append(encoded, '\n')

	temporary, err := os.CreateTemp(root, ".entry-pending-*.tmp")
	if err != nil {
		return fmt.Errorf("create entry pending operation: %w", err)
	}

	temporaryPath := temporary.Name()
	if err := writePendingFile(temporary, encoded); err != nil {
		return discardPendingTemporary(temporary, temporaryPath, err)
	}

	if err := temporary.Close(); err != nil {
		return errors.Join(fmt.Errorf("close entry pending operation: %w", err), removeFile(temporaryPath))
	}

	if err := os.Rename(temporaryPath, pendingPath); err != nil {
		return errors.Join(fmt.Errorf("publish entry pending operation: %w", err), removeFile(temporaryPath))
	}

	return syncDirectory(root)
}

func readEntryPendingOperation(path string) (entryPendingOperation, error) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return entryPendingOperation{}, fmt.Errorf("read entry pending operation: %w", err)
	}

	if len(encoded) > maxEventLineSize*2 {
		return entryPendingOperation{}, corruptEntryPending(path, 0, "pending operation exceeds the size limit")
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()

	var operation entryPendingOperation
	if err := decoder.Decode(&operation); err != nil {
		return entryPendingOperation{}, corruptEntryPending(path, 0, "decode pending operation: %v", err)
	}

	if err := requireJSONEnd(decoder); err != nil {
		return entryPendingOperation{}, corruptEntryPending(path, 0, "%v", err)
	}

	return operation, nil
}

func findEntryPendingPath(root string) (string, bool, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false, fmt.Errorf("inspect entry pending directory: %w", err)
	}

	var pendingPath string

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return "", false, corruptEntryPending(filepath.Join(root, entry.Name()), 0, "unexpected pending artifact")
		}

		if pendingPath != "" {
			return "", false, corruptEntryPending(root, 0, "multiple pending operations")
		}

		pendingPath = filepath.Join(root, entry.Name())
	}

	return pendingPath, pendingPath != "", nil
}

func entryAppendSeparator(segmentPath string, size int64) ([]byte, error) {
	if size == 0 {
		return nil, nil
	}

	file, err := os.Open(segmentPath)
	if err != nil {
		return nil, fmt.Errorf("open entry segment %q: %w", segmentPath, err)
	}

	lastByte := []byte{0}
	_, readErr := file.ReadAt(lastByte, size-1)

	closeErr := file.Close()
	if err := errors.Join(readErr, closeErr); err != nil {
		return nil, fmt.Errorf("inspect entry segment %q boundary: %w", segmentPath, err)
	}

	if lastByte[0] == '\n' {
		return nil, nil
	}

	return []byte{'\n'}, nil
}

func clearEntryPendingOperation(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove entry pending operation: %w", err)
	}

	return syncDirectory(filepath.Dir(path))
}

func corruptEntryPending(path string, offset int64, format string, arguments ...any) error {
	message := fmt.Sprintf(format, arguments...)

	return fmt.Errorf("%w: %s at byte %d: %s", ErrEntryJournalCorrupt, path, offset, message)
}

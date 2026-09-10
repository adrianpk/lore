package lore

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const pendingSchemaVersion = "lore/pending/v1"

type pendingOperation struct {
	SchemaVersion string `json:"schema_version"`
	Segment       string `json:"segment"`
	OriginalSize  int64  `json:"original_size"`
	Line          string `json:"line"`
}

func publishEvent(config Config, segmentPath string, line []byte) error {
	operation, err := preparePendingOperation(config.JournalRoot, segmentPath, line)
	if err != nil {
		return err
	}

	if err := writePendingOperation(config.PendingPath, operation); err != nil {
		return err
	}

	_, err = recoverPending(config)

	return err
}

func preparePendingOperation(root, segmentPath string, line []byte) (pendingOperation, error) {
	relative, err := filepath.Rel(root, segmentPath)
	if err != nil {
		return pendingOperation{}, fmt.Errorf("prepare pending operation: %w", err)
	}

	if !validRelativeSegment(relative) {
		return pendingOperation{}, fmt.Errorf("prepare pending operation: segment is outside the journal root")
	}

	originalSize, err := segmentSize(segmentPath)
	if err != nil {
		return pendingOperation{}, err
	}

	if err := validateSegmentBoundary(segmentPath, originalSize); err != nil {
		return pendingOperation{}, err
	}

	operation := pendingOperation{
		SchemaVersion: pendingSchemaVersion,
		Segment:       filepath.ToSlash(relative),
		OriginalSize:  originalSize,
		Line:          string(line),
	}
	if _, _, err := validatePendingOperation(root, operation); err != nil {
		return pendingOperation{}, err
	}

	return operation, nil
}

func recoverPending(config Config) (bool, error) {
	operation, exists, err := readPendingOperation(config.PendingPath)
	if err != nil || !exists {
		return false, err
	}

	segmentPath, line, err := validatePendingOperation(config.JournalRoot, operation)
	if err != nil {
		return false, err
	}

	if err := recoverPendingOperation(config.PendingPath, segmentPath, operation.OriginalSize, line); err != nil {
		return false, err
	}

	return true, nil
}

func recoverPendingOperation(pendingPath, segmentPath string, originalSize int64, line []byte) error {
	currentSize, err := segmentSize(segmentPath)
	if err != nil {
		return err
	}

	if currentSize < originalSize {
		return corruptPending(segmentPath, currentSize, "segment is shorter than its recorded original size")
	}

	if err := validateSegmentBoundary(segmentPath, originalSize); err != nil {
		return err
	}

	appendedSize := currentSize - originalSize
	if appendedSize > int64(len(line)) {
		return corruptPending(segmentPath, originalSize, "segment contains bytes beyond the pending event")
	}

	if appendedSize == 0 {
		return appendAndClearPending(pendingPath, segmentPath, line)
	}

	suffix, err := readSegmentRange(segmentPath, originalSize, appendedSize)
	if err != nil {
		return err
	}

	if bytes.Equal(suffix, line) {
		return clearPendingOperation(pendingPath)
	}

	if bytes.Equal(suffix, line[:len(suffix)]) {
		if err := truncateSegment(segmentPath, originalSize); err != nil {
			return err
		}

		return appendAndClearPending(pendingPath, segmentPath, line)
	}

	return corruptPending(segmentPath, originalSize, "terminal bytes do not match the pending event")
}

func appendAndClearPending(pendingPath, segmentPath string, line []byte) error {
	if err := appendEventLine(segmentPath, line); err != nil {
		return err
	}

	return clearPendingOperation(pendingPath)
}

func validatePendingOperation(root string, operation pendingOperation) (string, []byte, error) {
	relative, line, err := validatePendingMetadata(operation)
	if err != nil {
		return "", nil, err
	}

	event, err := decodePendingEvent(operation, line)
	if err != nil {
		return "", nil, err
	}

	segmentPath := filepath.Join(root, relative)
	if SegmentPath(root, event.RecordedAt) != segmentPath {
		return "", nil, corruptPending(operation.Segment, operation.OriginalSize, "pending event belongs to another segment")
	}

	return segmentPath, line, nil
}

func validatePendingMetadata(operation pendingOperation) (string, []byte, error) {
	if operation.SchemaVersion != pendingSchemaVersion {
		return "", nil, corruptPending(operation.Segment, 0, "unsupported pending schema version")
	}

	relative := filepath.FromSlash(operation.Segment)
	if !validRelativeSegment(relative) {
		return "", nil, corruptPending(operation.Segment, 0, "invalid pending segment path")
	}

	if operation.OriginalSize < 0 {
		return "", nil, corruptPending(operation.Segment, 0, "negative original segment size")
	}

	line := []byte(operation.Line)
	if len(line) == 0 || len(line) > maxEventLineSize || line[len(line)-1] != '\n' {
		return "", nil, corruptPending(operation.Segment, operation.OriginalSize, "invalid pending event framing")
	}

	if bytes.Count(line, []byte{'\n'}) != 1 {
		return "", nil, corruptPending(operation.Segment, operation.OriginalSize, "pending event contains an embedded newline")
	}

	return relative, line, nil
}

func decodePendingEvent(operation pendingOperation, line []byte) (Event, error) {
	event, err := decodeCanonicalActivity(line[:len(line)-1])
	if err != nil {
		return Event{}, corruptPending(operation.Segment, operation.OriginalSize, "decode pending event: %v", err)
	}

	canonical, err := canonicalActivityLine(event)
	if err != nil {
		return Event{}, fmt.Errorf("encode pending event: %w", err)
	}

	if !bytes.Equal(line, canonical) {
		return Event{}, corruptPending(operation.Segment, operation.OriginalSize, "pending event is not canonical JSON")
	}

	return event, nil
}

func validRelativeSegment(relative string) bool {
	if relative == "" || relative == "." || filepath.IsAbs(relative) {
		return false
	}

	if filepath.Clean(relative) != relative {
		return false
	}

	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func writePendingOperation(pendingPath string, operation pendingOperation) error {
	if err := ensurePendingAbsent(pendingPath); err != nil {
		return err
	}

	encoded, err := json.Marshal(operation)
	if err != nil {
		return fmt.Errorf("encode pending operation: %w", err)
	}

	encoded = append(encoded, '\n')

	temporary, err := os.CreateTemp(filepath.Dir(pendingPath), ".pending-*.tmp")
	if err != nil {
		return fmt.Errorf("create pending operation: %w", err)
	}

	temporaryPath := temporary.Name()
	if err := writePendingFile(temporary, encoded); err != nil {
		return discardPendingTemporary(temporary, temporaryPath, err)
	}

	if err := temporary.Close(); err != nil {
		return errors.Join(fmt.Errorf("close pending operation: %w", err), removeFile(temporaryPath))
	}

	if err := os.Rename(temporaryPath, pendingPath); err != nil {
		return errors.Join(fmt.Errorf("publish pending operation: %w", err), removeFile(temporaryPath))
	}

	return syncDirectory(filepath.Dir(pendingPath))
}

func writePendingFile(file *os.File, encoded []byte) error {
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("set pending operation permissions: %w", err)
	}

	if _, err := file.Write(encoded); err != nil {
		return fmt.Errorf("write pending operation: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync pending operation: %w", err)
	}

	return nil
}

func discardPendingTemporary(file *os.File, temporaryPath string, operationErr error) error {
	return errors.Join(operationErr, file.Close(), removeFile(temporaryPath))
}

func ensurePendingAbsent(pendingPath string) error {
	_, err := os.Lstat(pendingPath)
	if err == nil {
		return fmt.Errorf("write pending operation: pending operation already exists")
	}

	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect pending operation: %w", err)
	}

	return nil
}

func readPendingOperation(pendingPath string) (pendingOperation, bool, error) {
	info, err := os.Lstat(pendingPath)
	if errors.Is(err, os.ErrNotExist) {
		return pendingOperation{}, false, nil
	}

	if err != nil {
		return pendingOperation{}, false, fmt.Errorf("inspect pending operation: %w", err)
	}

	if !info.Mode().IsRegular() {
		return pendingOperation{}, false, corruptPending(pendingPath, 0, "pending operation is not a regular file")
	}

	encoded, err := os.ReadFile(pendingPath)
	if err != nil {
		return pendingOperation{}, false, fmt.Errorf("read pending operation: %w", err)
	}

	if len(encoded) > maxEventLineSize*2 {
		return pendingOperation{}, false, corruptPending(pendingPath, 0, "pending operation exceeds the size limit")
	}

	var operation pendingOperation

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&operation); err != nil {
		return pendingOperation{}, false, corruptPending(pendingPath, 0, "decode pending operation: %v", err)
	}

	if err := requireJSONEnd(decoder); err != nil {
		return pendingOperation{}, false, corruptPending(pendingPath, 0, "%v", err)
	}

	return operation, true, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any

	err := decoder.Decode(&extra)

	if !errors.Is(err, io.EOF) {
		return fmt.Errorf("pending operation contains trailing JSON")
	}

	return nil
}

func clearPendingOperation(pendingPath string) error {
	if err := os.Remove(pendingPath); err != nil {
		return fmt.Errorf("remove pending operation: %w", err)
	}

	return nil
}

func removeFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func segmentSize(segmentPath string) (int64, error) {
	info, err := os.Stat(segmentPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}

	if err != nil {
		return 0, fmt.Errorf("stat journal segment %q: %w", segmentPath, err)
	}

	if !info.Mode().IsRegular() {
		return 0, corruptPending(segmentPath, 0, "journal segment is not a regular file")
	}

	return info.Size(), nil
}

func validateSegmentTail(file *os.File, segmentPath string) error {
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat journal segment %q: %w", segmentPath, err)
	}

	return validateSegmentBoundaryFile(file, segmentPath, info.Size())
}

func validateSegmentBoundary(segmentPath string, size int64) error {
	if size == 0 {
		return nil
	}

	file, err := os.Open(segmentPath)
	if err != nil {
		return fmt.Errorf("open journal segment %q: %w", segmentPath, err)
	}

	boundaryErr := validateSegmentBoundaryFile(file, segmentPath, size)

	return errors.Join(boundaryErr, file.Close())
}

func validateSegmentBoundaryFile(file *os.File, segmentPath string, size int64) error {
	if size == 0 {
		return nil
	}

	lastByte := []byte{0}
	if _, err := file.ReadAt(lastByte, size-1); err != nil {
		return fmt.Errorf("read journal segment %q at byte %d: %w", segmentPath, size-1, err)
	}

	if lastByte[0] != '\n' {
		return corruptPending(segmentPath, size-1, "journal segment does not end with LF")
	}

	return nil
}

func readSegmentRange(segmentPath string, offset, size int64) ([]byte, error) {
	file, err := os.Open(segmentPath)
	if err != nil {
		return nil, fmt.Errorf("open journal segment %q: %w", segmentPath, err)
	}

	data := make([]byte, size)

	_, readErr := file.ReadAt(data, offset)
	if readErr != nil {
		readErr = fmt.Errorf("read journal segment %q at byte %d: %w", segmentPath, offset, readErr)
	}

	return data, errors.Join(readErr, file.Close())
}

func truncateSegment(segmentPath string, size int64) error {
	file, err := os.OpenFile(segmentPath, os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open journal segment %q for recovery: %w", segmentPath, err)
	}

	if err := file.Truncate(size); err != nil {
		return closeJournalFile(file, fmt.Errorf("truncate journal segment %q: %w", segmentPath, err))
	}

	if err := file.Sync(); err != nil {
		return closeJournalFile(file, fmt.Errorf("sync journal segment %q: %w", segmentPath, err))
	}

	return closeJournalFile(file, nil)
}

func syncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open directory %q for sync: %w", directory, err)
	}

	syncErr := file.Sync()
	if syncErr != nil {
		syncErr = fmt.Errorf("sync directory %q: %w", directory, syncErr)
	}

	return errors.Join(syncErr, file.Close())
}

func corruptPending(segmentPath string, offset int64, format string, arguments ...any) error {
	message := fmt.Sprintf(format, arguments...)

	return fmt.Errorf("%w: %s at byte %d: %s", ErrJournalCorrupt, segmentPath, offset, message)
}

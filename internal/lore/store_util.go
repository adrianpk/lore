package lore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxEventLineSize = 8 << 20

func listSegmentPaths(root string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			paths = append(paths, current)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list entry segments: %w", err)
	}

	sort.Strings(paths)

	return paths, nil
}

func appendEventLine(segmentPath string, line []byte) error {
	if err := os.MkdirAll(filepath.Dir(segmentPath), 0o700); err != nil {
		return fmt.Errorf("create entry segment directory: %w", err)
	}

	if err := os.Chmod(filepath.Dir(segmentPath), 0o700); err != nil {
		return fmt.Errorf("set entry segment directory permissions: %w", err)
	}

	file, err := os.OpenFile(segmentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open entry segment %q: %w", segmentPath, err)
	}

	if err := file.Chmod(0o600); err != nil {
		return closeStoreFile(file, fmt.Errorf("set entry segment permissions %q: %w", segmentPath, err))
	}

	if _, err := file.Write(line); err != nil {
		return closeStoreFile(file, fmt.Errorf("append entry segment %q: %w", segmentPath, err))
	}

	if err := file.Sync(); err != nil {
		return closeStoreFile(file, fmt.Errorf("sync entry segment %q: %w", segmentPath, err))
	}

	return closeStoreFile(file, nil)
}

func closeStoreFile(file *os.File, operationErr error) error {
	return errors.Join(operationErr, file.Close())
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

func discardPendingTemporary(file *os.File, path string, operationErr error) error {
	return errors.Join(operationErr, file.Close(), removeFile(path))
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any

	err := decoder.Decode(&extra)
	if !errors.Is(err, io.EOF) {
		return fmt.Errorf("input contains trailing JSON")
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
		return 0, fmt.Errorf("stat entry segment %q: %w", segmentPath, err)
	}

	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("%w: entry segment %q is not a regular file", ErrEntryJournalCorrupt, segmentPath)
	}

	return info.Size(), nil
}

func readSegmentRange(segmentPath string, offset, size int64) ([]byte, error) {
	file, err := os.Open(segmentPath)
	if err != nil {
		return nil, fmt.Errorf("open entry segment %q: %w", segmentPath, err)
	}

	data := make([]byte, size)

	_, readErr := file.ReadAt(data, offset)
	if readErr != nil {
		readErr = fmt.Errorf("read entry segment %q at byte %d: %w", segmentPath, offset, readErr)
	}

	return data, errors.Join(readErr, file.Close())
}

func truncateSegment(segmentPath string, size int64) error {
	file, err := os.OpenFile(segmentPath, os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open entry segment %q for recovery: %w", segmentPath, err)
	}

	if err := file.Truncate(size); err != nil {
		return closeStoreFile(file, fmt.Errorf("truncate entry segment %q: %w", segmentPath, err))
	}

	if err := file.Sync(); err != nil {
		return closeStoreFile(file, fmt.Errorf("sync entry segment %q: %w", segmentPath, err))
	}

	return closeStoreFile(file, nil)
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

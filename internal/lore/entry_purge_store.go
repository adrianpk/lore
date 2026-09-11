package lore

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	purgeStagingDirectory = ".entries-purge-staging"
	purgeBackupDirectory  = ".entries-purge-backup"
)

func entryStoreDigest(ctx context.Context, root string) ([sha256.Size]byte, error) {
	hash := sha256.New()

	paths, err := listSegmentPaths(root)
	if err != nil {
		return [sha256.Size]byte{}, err
	}

	for _, path := range paths {
		if err := hashEntrySegment(ctx, hash, root, path); err != nil {
			return [sha256.Size]byte{}, err
		}
	}

	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))

	return digest, nil
}

func hashEntrySegment(ctx context.Context, writer io.Writer, root, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	relative, err := filepath.Rel(root, path)
	if err != nil {
		return fmt.Errorf("resolve entry segment digest path: %w", err)
	}

	if _, err := io.WriteString(writer, filepath.ToSlash(relative)+"\x00"); err != nil {
		return fmt.Errorf("hash entry segment path: %w", err)
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open entry segment for digest: %w", err)
	}

	_, copyErr := io.Copy(writer, file)
	if copyErr == nil {
		_, copyErr = io.WriteString(writer, "\x00")
	}

	return errors.Join(copyErr, file.Close())
}

func rewriteEntryStore(ctx context.Context, config Config, index *entryIndex, purgedRoots map[EntryID]struct{}) error {
	staging, backup := purgeWorkPaths(config)
	if err := requirePurgeWorkPathsAbsent(staging, backup); err != nil {
		return err
	}

	if err := os.Mkdir(staging, 0o700); err != nil {
		return fmt.Errorf("create purge staging directory: %w", err)
	}

	if err := writeRetainedEntries(ctx, staging, index, purgedRoots); err != nil {
		return errors.Join(err, removePurgeTree(staging))
	}

	if err := swapEntryDirectories(config, staging, backup); err != nil {
		return err
	}

	return removePurgeTree(backup)
}

func writeRetainedEntries(ctx context.Context, root string, index *entryIndex, purgedRoots map[EntryID]struct{}) error {
	for _, id := range index.order {
		if err := ctx.Err(); err != nil {
			return err
		}

		if _, purge := purgedRoots[index.roots[id]]; purge {
			continue
		}

		entry := index.entries[id]

		line, err := canonicalEntryLine(entry)
		if err != nil {
			return err
		}

		if err := appendEventLine(EntrySegmentPath(root, entry.RecordedAt), line); err != nil {
			return err
		}
	}

	return syncDirectory(root)
}

func swapEntryDirectories(config Config, staging, backup string) error {
	if err := os.Rename(config.EntriesRoot, backup); err != nil {
		return fmt.Errorf("stage current entry store for purge: %w", err)
	}

	if err := syncDirectory(config.DataRoot); err != nil {
		return rollbackEntryDirectory(config, staging, backup, err)
	}

	if err := os.Rename(staging, config.EntriesRoot); err != nil {
		return rollbackEntryDirectory(config, staging, backup, fmt.Errorf("publish purged entry store: %w", err))
	}

	return syncDirectory(config.DataRoot)
}

func rollbackEntryDirectory(config Config, staging, backup string, operationErr error) error {
	restoreErr := os.Rename(backup, config.EntriesRoot)
	if restoreErr != nil {
		restoreErr = fmt.Errorf("restore entry store after purge failure: %w", restoreErr)
	}

	return errors.Join(operationErr, restoreErr, removePurgeTree(staging), syncDirectory(config.DataRoot))
}

func recoverEntryPurge(config Config) error {
	staging, backup := purgeWorkPaths(config)

	liveExists, err := directoryExists(config.EntriesRoot)
	if err != nil {
		return err
	}

	stagingExists, err := directoryExists(staging)
	if err != nil {
		return err
	}

	backupExists, err := directoryExists(backup)
	if err != nil {
		return err
	}

	return recoverEntryPurgeState(config, liveExists, stagingExists, backupExists)
}

func recoverEntryPurgeState(config Config, live, staging, backup bool) error {
	stagingPath, backupPath := purgeWorkPaths(config)

	switch purgeRecoveryState(live, staging, backup) {
	case 0, 1:
		return nil
	case 3:
		return removePurgeTree(stagingPath)
	case 6:
		if err := os.Rename(stagingPath, config.EntriesRoot); err != nil {
			return fmt.Errorf("recover published purge store: %w", err)
		}

		return removePurgeTree(backupPath)
	case 5:
		return removePurgeTree(backupPath)
	case 4:
		return os.Rename(backupPath, config.EntriesRoot)
	default:
		return fmt.Errorf("%w: ambiguous purge recovery state", ErrEntryJournalCorrupt)
	}
}

func purgeRecoveryState(live, staging, backup bool) int {
	state := 0
	if live {
		state |= 1
	}

	if staging {
		state |= 2
	}

	if backup {
		state |= 4
	}

	return state
}

func purgeWorkPaths(config Config) (string, string) {
	return filepath.Join(config.DataRoot, purgeStagingDirectory), filepath.Join(config.DataRoot, purgeBackupDirectory)
}

func requirePurgeWorkPathsAbsent(paths ...string) error {
	for _, path := range paths {
		exists, err := directoryExists(path)
		if err != nil {
			return err
		}

		if exists {
			return fmt.Errorf("%w: unresolved purge work directory %q", ErrEntryJournalCorrupt, path)
		}
	}

	return nil
}

func directoryExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("inspect purge path %q: %w", path, err)
	}

	if !info.IsDir() {
		return false, fmt.Errorf("%w: purge path %q is not a directory", ErrEntryJournalCorrupt, path)
	}

	return true, nil
}

func removePurgeTree(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove purge work directory %q: %w", path, err)
	}

	return syncDirectory(filepath.Dir(path))
}

package lore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
	"unicode/utf8"
)

// VerifyScope identifies the canonical segment set that was checked.
type VerifyScope string

const (
	// VerifyScopeActive checks the current UTC monthly segment.
	VerifyScopeActive VerifyScope = "active"
	// VerifyScopeFull checks every canonical segment and global invariant.
	VerifyScopeFull VerifyScope = "full"
)

// VerifyParams selects active or full journal verification.
type VerifyParams struct {
	Full bool `json:"full"`
}

// VerifyResult summarizes one successful read-only verification.
type VerifyResult struct {
	Scope        VerifyScope `json:"scope"`
	SegmentCount int         `json:"segment_count"`
	EventCount   int         `json:"event_count"`
}

// IntegrityError locates one canonical journal invariant failure.
type IntegrityError struct {
	Segment    string `json:"segment"`
	Line       int    `json:"line"`
	ByteOffset int64  `json:"byte_offset"`
	Reason     string `json:"reason"`
}

// Error describes the exact journal location and failed invariant.
func (integrityError *IntegrityError) Error() string {
	return fmt.Sprintf(
		"%s: %s:%d at byte %d: %s",
		ErrJournalCorrupt,
		integrityError.Segment,
		integrityError.Line,
		integrityError.ByteOffset,
		integrityError.Reason,
	)
}

// Unwrap identifies every integrity failure as journal corruption.
func (integrityError *IntegrityError) Unwrap() error {
	return ErrJournalCorrupt
}

// Verifier checks canonical journal files without modifying them.
type Verifier struct {
	journal *Journal
	now     func() time.Time
}

// NewVerifier constructs a verifier without performing IO.
func NewVerifier(journal *Journal) *Verifier {
	return &Verifier{journal: journal, now: time.Now}
}

// Verify checks the selected canonical journal scope.
func (verifier *Verifier) Verify(ctx context.Context, params VerifyParams) (VerifyResult, error) {
	verifier.journal.mu.RLock()
	defer verifier.journal.mu.RUnlock()

	if params.Full {
		result, err := verifier.verifyFull(ctx)
		if err == nil && !verifier.journal.loaded && verifier.journal.loadErr != nil {
			return VerifyResult{}, verifier.journal.loadErr
		}

		return result, err
	}

	if !verifier.journal.loaded {
		return VerifyResult{}, ErrJournalNotLoaded
	}

	return verifier.verifyActive(ctx)
}

func (verifier *Verifier) verifyFull(ctx context.Context) (VerifyResult, error) {
	paths, err := listSegmentPaths(verifier.journal.config.JournalRoot)
	if err != nil {
		return VerifyResult{}, err
	}

	return verifyPaths(ctx, verifier.journal.config.JournalRoot, paths, newJournalIndex(), VerifyScopeFull)
}

func (verifier *Verifier) verifyActive(ctx context.Context) (VerifyResult, error) {
	segmentPath := SegmentPath(verifier.journal.config.JournalRoot, verifier.now().UTC())

	exists, err := regularFileExists(segmentPath)
	if err != nil {
		return VerifyResult{}, err
	}

	if !exists {
		return VerifyResult{Scope: VerifyScopeActive}, nil
	}

	index := verifier.indexBefore(segmentPath)

	return verifyPaths(
		ctx,
		verifier.journal.config.JournalRoot,
		[]string{segmentPath},
		index,
		VerifyScopeActive,
	)
}

func (verifier *Verifier) indexBefore(segmentPath string) *journalIndex {
	index := newJournalIndex()

	for _, id := range verifier.journal.order {
		event := verifier.journal.events[id]
		if SegmentPath(verifier.journal.config.JournalRoot, event.RecordedAt) >= segmentPath {
			continue
		}

		addActivityToIndex(event, index)
	}

	return index
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("inspect journal segment %q: %w", path, err)
	}

	if !info.Mode().IsRegular() {
		return false, integrityFailure(path, 0, 0, "journal segment is not a regular file")
	}

	return true, nil
}

func verifyPaths(ctx context.Context, root string, paths []string, index *journalIndex, scope VerifyScope) (VerifyResult, error) {
	initialCount := len(index.events)

	for _, segmentPath := range paths {
		if err := verifySegment(ctx, root, segmentPath, index); err != nil {
			return VerifyResult{}, err
		}
	}

	return VerifyResult{
		Scope:        scope,
		SegmentCount: len(paths),
		EventCount:   len(index.events) - initialCount,
	}, nil
}

func verifySegment(ctx context.Context, root, segmentPath string, index *journalIndex) (resultErr error) {
	exists, err := regularFileExists(segmentPath)
	if err != nil {
		return err
	}

	if !exists {
		return integrityFailure(segmentPath, 0, 0, "journal segment disappeared during verification")
	}

	file, err := os.Open(segmentPath)
	if err != nil {
		return fmt.Errorf("open journal segment %q: %w", segmentPath, err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && resultErr == nil {
			resultErr = fmt.Errorf("close journal segment %q: %w", segmentPath, closeErr)
		}
	}()

	return verifySegmentContents(ctx, root, segmentPath, bufio.NewReader(file), index)
}

func verifySegmentContents(ctx context.Context, root, segmentPath string, reader *bufio.Reader, index *journalIndex) error {
	lineNumber := 0
	byteOffset := int64(0)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		line, readErr := reader.ReadBytes('\n')
		if len(line) == 0 && errors.Is(readErr, io.EOF) {
			return nil
		}

		lineNumber++
		if errors.Is(readErr, io.EOF) {
			return integrityFailure(segmentPath, lineNumber, byteOffset+int64(len(line))-1, "line does not end with LF")
		}

		if readErr != nil {
			return fmt.Errorf("read journal segment %q at byte %d: %w", segmentPath, byteOffset, readErr)
		}

		if err := verifyLine(root, segmentPath, lineNumber, byteOffset, line, index); err != nil {
			return err
		}

		byteOffset += int64(len(line))
	}
}

func verifyLine(root, segmentPath string, lineNumber int, byteOffset int64, framedLine []byte, index *journalIndex) error {
	line := bytes.TrimSuffix(framedLine, []byte{'\n'})
	if len(line) == 0 {
		return integrityFailure(segmentPath, lineNumber, byteOffset, "blank line")
	}

	if len(line) > maxEventLineSize {
		return integrityFailure(segmentPath, lineNumber, byteOffset+maxEventLineSize, "event exceeds the size limit")
	}

	if !utf8.Valid(line) {
		return integrityFailure(segmentPath, lineNumber, byteOffset+invalidUTF8Offset(line), "event is not valid UTF-8")
	}

	event, err := decodeCanonicalActivity(line)
	if err != nil {
		return integrityFailure(segmentPath, lineNumber, byteOffset+jsonErrorOffset(err), "decode event: %v", err)
	}

	if SegmentPath(root, event.RecordedAt) != segmentPath {
		return integrityFailure(segmentPath, lineNumber, byteOffset, "event belongs to %q", SegmentPath(root, event.RecordedAt))
	}

	if err := validateActivityAgainstIndex(event, index); err != nil {
		return integrityFailure(segmentPath, lineNumber, byteOffset, "%v", err)
	}

	addActivityToIndex(event, index)

	return nil
}

func invalidUTF8Offset(value []byte) int64 {
	for offset := 0; offset < len(value); {
		_, size := utf8.DecodeRune(value[offset:])
		if size == 1 && value[offset] >= utf8.RuneSelf {
			return int64(offset)
		}

		offset += size
	}

	return 0
}

func jsonErrorOffset(err error) int64 {
	var syntaxError *json.SyntaxError
	if errors.As(err, &syntaxError) && syntaxError.Offset > 0 {
		return syntaxError.Offset - 1
	}

	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) && typeError.Offset > 0 {
		return typeError.Offset - 1
	}

	return 0
}

func integrityFailure(segment string, line int, byteOffset int64, format string, arguments ...any) error {
	return &IntegrityError{
		Segment:    segment,
		Line:       line,
		ByteOffset: byteOffset,
		Reason:     fmt.Sprintf(format, arguments...),
	}
}

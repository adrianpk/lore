package lore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAcceptanceCrossSegmentCorrectionSurvivesRestart(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("load journal: %v", err)
	}

	milestone := validMilestoneEvent()
	milestone.RecordedAt = time.Date(2026, 8, 31, 23, 59, 0, 0, time.UTC)
	milestone.ID = nextEventID(t, milestone.RecordedAt, 31)
	milestone.Provenance = &ActivityProvenance{Type: ProvenanceTypeAgent, Harness: "codex", Model: "provider/model"}
	appendJournalEvent(t, journal, milestone)

	corrector := NewCorrector(journal)
	corrector.now = func() time.Time {
		return time.Date(2026, 9, 1, 0, 1, 0, 0, time.UTC)
	}
	corrector.entropy = bytes.NewReader(bytes.Repeat([]byte{32}, 10))

	corrected, err := corrector.Correct(context.Background(), CorrectParams{
		TargetID: milestone.ID,
		Reason:   "Clarify cross-segment behavior",
	})
	if err != nil {
		t.Fatalf("correct milestone: %v", err)
	}

	if SegmentPath(config.JournalRoot, milestone.RecordedAt) == SegmentPath(config.JournalRoot, corrected.Event.RecordedAt) {
		t.Fatal("milestone and correction use the same segment")
	}

	reloaded := NewJournal(config)
	if err := reloaded.Load(context.Background()); err != nil {
		t.Fatalf("reload journal: %v", err)
	}

	assertAcceptanceCorrection(t, reloaded, milestone, corrected.Event)
	assertAcceptanceServiceView(t, reloaded, corrected.Event)
}

func TestAcceptancePendingRecoveryConcurrentWritesAndRestart(t *testing.T) {
	t.Parallel()

	config := testJournalConfig(t)
	if err := prepareJournalDirectories(config); err != nil {
		t.Fatalf("prepare journal: %v", err)
	}

	pendingEvent := validMilestoneEvent()

	segmentPath, line, operation := prepareTestPending(t, config, pendingEvent)
	if err := writePendingOperation(config.PendingPath, operation); err != nil {
		t.Fatalf("write pending operation: %v", err)
	}

	writePartialPendingTail(t, segmentPath, line)

	journal := NewJournal(config)
	if err := journal.Load(context.Background()); err != nil {
		t.Fatalf("recover journal: %v", err)
	}

	appendAcceptanceEventsConcurrently(t, journal, 24)
	assertFileMissing(t, config.PendingPath)

	reloaded := NewJournal(config)
	if err := reloaded.Load(context.Background()); err != nil {
		t.Fatalf("restart journal: %v", err)
	}

	if reloaded.Count() != 25 {
		t.Fatalf("reloaded count = %d, want 25", reloaded.Count())
	}

	result, err := NewVerifier(reloaded).Verify(context.Background(), VerifyParams{Full: true})
	if err != nil {
		t.Fatalf("verify recovered journal: %v", err)
	}

	if result.EventCount != 25 {
		t.Fatalf("verification result = %#v", result)
	}
}

func TestAcceptanceIPCLimitsRemainStable(t *testing.T) {
	t.Parallel()

	oversizeParams := struct {
		Value string `json:"value"`
	}{Value: strings.Repeat("x", MaxRequestSize)}

	if _, err := encodeClientRequest("record", oversizeParams); err == nil {
		t.Fatal("expected oversized request error")
	}

	handler := RequestHandlerFunc(func(context.Context, Request) Response {
		return SuccessResponse(struct {
			Value string `json:"value"`
		}{Value: strings.Repeat("x", MaxResponseSize)})
	})
	client, _ := startTransportServer(t, handler)

	err := client.Call(context.Background(), "large", struct{}{}, nil)

	var serviceError *ServiceError
	if !errors.As(err, &serviceError) || serviceError.Code != ErrorInternal {
		t.Fatalf("oversized response error = %v", err)
	}
}

func assertAcceptanceCorrection(t *testing.T, journal *Journal, milestone, correction Event) {
	t.Helper()

	result, err := journal.Search(context.Background(), Query{
		Harness: "codex",
		Model:   "provider/model",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("search execution provenance: %v", err)
	}

	if len(result.Events) != 1 || result.Events[0].ID != correction.ID {
		t.Fatalf("effective events = %#v", result.Events)
	}

	shown, err := journal.Show(context.Background(), ShowParams{ID: milestone.ID})
	if err != nil {
		t.Fatalf("show effective correction: %v", err)
	}

	effective := shown.Event
	if effective.Supersedes != milestone.ID || *effective.Scope != *milestone.Scope || !reflect.DeepEqual(*effective.Properties, *milestone.Properties) {
		t.Fatalf("self-contained correction = %#v", effective)
	}

	if effective.Provenance == milestone.Provenance || *effective.Provenance != *milestone.Provenance {
		t.Fatalf("correction provenance = %#v", effective.Provenance)
	}
}

func assertAcceptanceServiceView(t *testing.T, journal *Journal, correction Event) {
	t.Helper()

	client, _ := startTransportServer(t, NewService(NewRecorder(journal)))

	var searchResult QueryResult
	if err := client.Call(context.Background(), "search", Query{Text: "cross-segment", Limit: 10}, &searchResult); err != nil {
		t.Fatalf("service search: %v", err)
	}

	if len(searchResult.Events) != 1 || searchResult.Events[0].ID != correction.ID {
		t.Fatalf("service search result = %#v", searchResult)
	}

	var verifyResult VerifyResult
	if err := client.Call(context.Background(), "verify", VerifyParams{Full: true}, &verifyResult); err != nil {
		t.Fatalf("service verify: %v", err)
	}

	if verifyResult.SegmentCount != 2 || verifyResult.EventCount != 2 {
		t.Fatalf("service verification result = %#v", verifyResult)
	}
}

func appendAcceptanceEventsConcurrently(t *testing.T, journal *Journal, count int) {
	t.Helper()

	errorsChannel := make(chan error, count)

	var waitGroup sync.WaitGroup
	waitGroup.Add(count)

	for sequence := 1; sequence <= count; sequence++ {
		event := distinctMilestoneEvent(t, sequence+100)

		go func() {
			defer waitGroup.Done()

			_, err := journal.Append(context.Background(), event)
			errorsChannel <- err
		}()
	}

	waitGroup.Wait()
	close(errorsChannel)

	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("concurrent append: %v", err)
		}
	}
}

func TestAcceptanceVerificationIsReadOnly(t *testing.T) {
	t.Parallel()

	journal, verifier, event := newSingleEventVerifier(t)
	segmentPath := SegmentPath(journal.config.JournalRoot, event.RecordedAt)

	before, err := os.ReadFile(segmentPath)
	if err != nil {
		t.Fatalf("read segment before verification: %v", err)
	}

	if _, err := verifier.Verify(context.Background(), VerifyParams{Full: true}); err != nil {
		t.Fatalf("verify journal: %v", err)
	}

	after, err := os.ReadFile(segmentPath)
	if err != nil {
		t.Fatalf("read segment after verification: %v", err)
	}

	if !bytes.Equal(before, after) {
		t.Fatal("verification changed canonical segment content")
	}
}

package lore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestServiceHandlesRecord(t *testing.T) {
	t.Parallel()

	recorder, _ := testRecorder(t)
	service := NewService(recorder)

	encoded, err := json.Marshal(validRecordParams())
	if err != nil {
		t.Fatalf("marshal record params: %v", err)
	}

	response := service.Handle(context.Background(), Request{
		Protocol:  ProtocolVersion,
		Operation: "record",
		Params:    encoded,
	})
	if !response.OK || response.Error != nil {
		t.Fatalf("Handle() response = %#v", response)
	}

	var result RecordResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatalf("decode record result: %v", err)
	}

	if !result.Recorded || result.ID == "" {
		t.Fatalf("record result = %#v", result)
	}
}

func TestServiceRejectsInvalidRequest(t *testing.T) {
	t.Parallel()

	recorder, _ := testRecorder(t)
	service := NewService(recorder)

	tests := []struct {
		name      string
		operation string
		params    json.RawMessage
		wantCode  ErrorCode
	}{
		{
			name:      "unknown operation",
			operation: "missing",
			params:    json.RawMessage(`{}`),
			wantCode:  ErrorUnknownOperation,
		},
		{
			name:      "malformed params",
			operation: "record",
			params:    json.RawMessage(`{"unknown":true}`),
			wantCode:  ErrorInvalidRequest,
		},
		{
			name:      "invalid event",
			operation: "record",
			params:    json.RawMessage(`{}`),
			wantCode:  ErrorInvalidEvent,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := service.Handle(context.Background(), Request{
				Protocol:  ProtocolVersion,
				Operation: test.operation,
				Params:    test.params,
			})

			if response.OK || response.Error == nil || response.Error.Code != test.wantCode {
				t.Fatalf("Handle() response = %#v, want code %q", response, test.wantCode)
			}
		})
	}
}

func TestRecordErrorResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode ErrorCode
	}{
		{name: "invalid", err: ErrEventInvalid, wantCode: ErrorInvalidEvent},
		{name: "corrupt", err: ErrJournalCorrupt, wantCode: ErrorJournalCorrupt},
		{name: "unavailable", err: ErrJournalNotLoaded, wantCode: ErrorJournalUnavailable},
		{name: "other", err: errors.New("storage failure"), wantCode: ErrorJournalUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := recordErrorResponse(test.err)
			if response.Error == nil || response.Error.Code != test.wantCode {
				t.Fatalf("recordErrorResponse() = %#v", response)
			}
		})
	}
}

func TestServiceHandlesRetrievalAndCorrection(t *testing.T) {
	t.Parallel()

	fixture := newQueryFixture(t)
	service := NewService(NewRecorder(fixture.journal))
	service.corrector = deterministicCorrector(fixture.journal)

	searchResponse := handleTestRequest(t, service, "search", Query{Text: "runtime", Limit: 10})

	var searchResult QueryResult
	decodeTestResult(t, searchResponse, &searchResult)

	if len(searchResult.Events) != 1 || searchResult.Events[0].ID != fixture.correction.ID {
		t.Fatalf("search result = %#v", searchResult)
	}

	showResponse := handleTestRequest(t, service, "show", ShowParams{ID: fixture.milestoneA.ID})

	var showResult ShowResult
	decodeTestResult(t, showResponse, &showResult)

	if showResult.Event.ID != fixture.correction.ID {
		t.Fatalf("show result = %#v", showResult)
	}

	reportResponse := handleTestRequest(t, service, "report", ReportParams{
		From:     "2026-08-05",
		To:       "2026-08-08",
		Timezone: "UTC",
	})

	var reportResult ReportResult
	decodeTestResult(t, reportResponse, &reportResult)

	if reportResult.ReturnedCount != 3 {
		t.Fatalf("report result = %#v", reportResult)
	}

	correctResponse := handleTestRequest(t, service, "correct", CorrectParams{
		TargetID: fixture.correction.ID,
		Reason:   "Clarify service behavior",
	})

	var correctResult CorrectResult
	decodeTestResult(t, correctResponse, &correctResult)

	if correctResult.Event.Supersedes != fixture.correction.ID {
		t.Fatalf("correct result = %#v", correctResult)
	}

	verifyResponse := handleTestRequest(t, service, "verify", VerifyParams{Full: true})

	var verifyResult VerifyResult
	decodeTestResult(t, verifyResponse, &verifyResult)

	if verifyResult.Scope != VerifyScopeFull || verifyResult.EventCount != 5 {
		t.Fatalf("verify result = %#v", verifyResult)
	}
}

func TestServiceMapsRetrievalAndCorrectionErrors(t *testing.T) {
	t.Parallel()

	fixture := newQueryFixture(t)
	service := NewService(NewRecorder(fixture.journal))

	tests := []struct {
		name      string
		operation string
		params    any
		wantCode  ErrorCode
	}{
		{
			name:      "invalid query",
			operation: "search",
			params:    Query{Limit: maximumQueryLimit + 1},
			wantCode:  ErrorInvalidRequest,
		},
		{
			name:      "missing event",
			operation: "show",
			params:    ShowParams{ID: "01K1Z8V7BMMN6W7Y6R8QJHA9ZZ"},
			wantCode:  ErrorNotFound,
		},
		{
			name:      "missing correction target",
			operation: "correct",
			params: CorrectParams{
				TargetID: "01K1Z8V7BMMN6W7Y6R8QJHA9ZZ",
				Reason:   "Missing target",
			},
			wantCode: ErrorNotFound,
		},
		{
			name:      "invalid report",
			operation: "report",
			params:    ReportParams{From: "2026-08-01", To: "2026-08-01", Timezone: "UTC"},
			wantCode:  ErrorInvalidRequest,
		},
		{
			name:      "stale correction target",
			operation: "correct",
			params: CorrectParams{
				TargetID: fixture.milestoneA.ID,
				Reason:   "Stale target",
			},
			wantCode: ErrorInvalidEvent,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := handleTestRequest(t, service, test.operation, test.params)
			if response.OK || response.Error == nil || response.Error.Code != test.wantCode {
				t.Fatalf("Handle() response = %#v, want code %q", response, test.wantCode)
			}
		})
	}
}

func TestServiceReturnsExactVerificationFailure(t *testing.T) {
	t.Parallel()

	journal, _, event := newSingleEventVerifier(t)
	segmentPath := SegmentPath(journal.config.JournalRoot, event.RecordedAt)
	corruptFirstByte(t, segmentPath)

	service := NewService(NewRecorder(journal))
	response := handleTestRequest(t, service, "verify", VerifyParams{Full: true})

	if response.OK || response.Error == nil || response.Error.Code != ErrorJournalCorrupt {
		t.Fatalf("Handle() response = %#v", response)
	}

	if !strings.Contains(response.Error.Message, segmentPath) || !strings.Contains(response.Error.Message, "byte 0") {
		t.Fatalf("verification message = %q", response.Error.Message)
	}
}

func handleTestRequest(t *testing.T, service *Service, operation string, params any) Response {
	t.Helper()

	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal %s params: %v", operation, err)
	}

	return service.Handle(context.Background(), Request{
		Protocol:  ProtocolVersion,
		Operation: operation,
		Params:    encoded,
	})
}

func decodeTestResult(t *testing.T, response Response, target any) {
	t.Helper()

	if !response.OK || response.Error != nil {
		t.Fatalf("Handle() response = %#v", response)
	}

	if err := json.Unmarshal(response.Result, target); err != nil {
		t.Fatalf("decode service result: %v", err)
	}
}

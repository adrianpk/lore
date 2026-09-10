package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
)

// Service dispatches validated protocol operations to application workflows.
type Service struct {
	recorder  *Recorder
	journal   *Journal
	corrector *Corrector
	verifier  *Verifier
}

// NewService constructs a service without performing IO.
func NewService(recorder *Recorder) *Service {
	return &Service{
		recorder:  recorder,
		journal:   recorder.journal,
		corrector: NewCorrector(recorder.journal),
		verifier:  NewVerifier(recorder.journal),
	}
}

// Handle executes one local service request.
func (service *Service) Handle(ctx context.Context, request Request) Response {
	switch request.Operation {
	case "correct":
		return service.handleCorrect(ctx, request.Params)
	case "record":
		return service.handleRecord(ctx, request.Params)
	case "report":
		return service.handleReport(ctx, request.Params)
	case "search":
		return service.handleSearch(ctx, request.Params)
	case "show":
		return service.handleShow(ctx, request.Params)
	case "verify":
		return service.handleVerify(ctx, request.Params)
	default:
		return ErrorResponse(ErrorUnknownOperation, "unknown operation")
	}
}

func (service *Service) handleRecord(ctx context.Context, encoded json.RawMessage) Response {
	var params RecordParams
	if err := decodeOperationParams(encoded, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode record params")
	}

	result, err := service.recorder.Record(ctx, params)
	if err != nil {
		return recordErrorResponse(err)
	}

	return SuccessResponse(result)
}

func (service *Service) handleSearch(ctx context.Context, encoded json.RawMessage) Response {
	var query Query
	if err := decodeOperationParams(encoded, &query); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode search params")
	}

	result, err := service.journal.Search(ctx, query)
	if err != nil {
		return retrievalErrorResponse(err)
	}

	return SuccessResponse(result)
}

func (service *Service) handleReport(ctx context.Context, encoded json.RawMessage) Response {
	var params ReportParams
	if err := decodeOperationParams(encoded, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode report params")
	}

	result, err := service.journal.Report(ctx, params)
	if err != nil {
		return retrievalErrorResponse(err)
	}

	return SuccessResponse(result)
}

func (service *Service) handleShow(ctx context.Context, encoded json.RawMessage) Response {
	var params ShowParams
	if err := decodeOperationParams(encoded, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode show params")
	}

	result, err := service.journal.Show(ctx, params)
	if err != nil {
		return retrievalErrorResponse(err)
	}

	return SuccessResponse(result)
}

func (service *Service) handleCorrect(ctx context.Context, encoded json.RawMessage) Response {
	var params CorrectParams
	if err := decodeOperationParams(encoded, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode correct params")
	}

	result, err := service.corrector.Correct(ctx, params)
	if err != nil {
		return correctionErrorResponse(err)
	}

	return SuccessResponse(result)
}

func (service *Service) handleVerify(ctx context.Context, encoded json.RawMessage) Response {
	var params VerifyParams
	if err := decodeOperationParams(encoded, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode verify params")
	}

	result, err := service.verifier.Verify(ctx, params)
	if err != nil {
		return verificationErrorResponse(err)
	}

	return SuccessResponse(result)
}

func decodeOperationParams(encoded json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	return requireJSONEnd(decoder)
}

func recordErrorResponse(err error) Response {
	switch {
	case errors.Is(err, ErrEventInvalid):
		return ErrorResponse(ErrorInvalidEvent, err.Error())
	case errors.Is(err, ErrJournalCorrupt):
		return ErrorResponse(ErrorJournalCorrupt, err.Error())
	case errors.Is(err, ErrJournalNotLoaded):
		return ErrorResponse(ErrorJournalUnavailable, "journal is unavailable")
	default:
		return ErrorResponse(ErrorJournalUnavailable, "journal operation failed")
	}
}

func retrievalErrorResponse(err error) Response {
	switch {
	case errors.Is(err, ErrQueryInvalid), errors.Is(err, ErrCommitAmbiguous), errors.Is(err, ErrReportInvalid):
		return ErrorResponse(ErrorInvalidRequest, err.Error())
	case errors.Is(err, ErrEventNotFound):
		return ErrorResponse(ErrorNotFound, err.Error())
	case errors.Is(err, ErrJournalCorrupt):
		return ErrorResponse(ErrorJournalCorrupt, err.Error())
	case errors.Is(err, ErrJournalNotLoaded):
		return ErrorResponse(ErrorJournalUnavailable, "journal is unavailable")
	default:
		return ErrorResponse(ErrorJournalUnavailable, "journal operation failed")
	}
}

func correctionErrorResponse(err error) Response {
	switch {
	case errors.Is(err, ErrCorrectionTargetNotFound):
		return ErrorResponse(ErrorNotFound, err.Error())
	case errors.Is(err, ErrEventInvalid), errors.Is(err, ErrCorrectionTargetStale):
		return ErrorResponse(ErrorInvalidEvent, err.Error())
	default:
		return recordErrorResponse(err)
	}
}

func verificationErrorResponse(err error) Response {
	switch {
	case errors.Is(err, ErrJournalCorrupt):
		return ErrorResponse(ErrorJournalCorrupt, err.Error())
	case errors.Is(err, ErrJournalNotLoaded):
		return ErrorResponse(ErrorJournalUnavailable, "journal is unavailable")
	default:
		return ErrorResponse(ErrorJournalUnavailable, "journal verification failed")
	}
}

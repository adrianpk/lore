package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
)

// VerifyEntriesParams configures one non-mutating journal verification.
type VerifyEntriesParams struct {
	Scheduled bool `json:"scheduled,omitempty"`
}

// VerifyEntriesResult reports the indexed revision and effective-entry counts.
type VerifyEntriesResult struct {
	RevisionCount  int `json:"revision_count"`
	EffectiveCount int `json:"effective_count"`
}

// Service dispatches validated protocol operations to knowledge workflows.
type Service struct {
	journal   *EntryJournal
	workflows *EntryWorkflows
}

// NewService constructs a service without performing IO.
func NewService(journal *EntryJournal) *Service {
	return &Service{journal: journal, workflows: NewEntryWorkflows(journal)}
}

// Handle executes one local service request.
func (service *Service) Handle(ctx context.Context, request Request) Response {
	switch request.Operation {
	case "archive":
		return service.handleLifecycle(ctx, request.Params, EntryStatusArchived)
	case "invalidate":
		return service.handleLifecycle(ctx, request.Params, EntryStatusInvalidated)
	case "record":
		return service.handleRecord(ctx, request.Params)
	case "restore":
		return service.handleLifecycle(ctx, request.Params, EntryStatusActive)
	case "revise":
		return service.handleRevise(ctx, request.Params)
	case "search":
		return service.handleSearch(request.Params)
	case "show":
		return service.handleShow(request.Params)
	case "verify":
		return service.handleVerify(request.Params)
	default:
		return service.handleMaintenance(ctx, request)
	}
}

func (service *Service) handleMaintenance(ctx context.Context, request Request) Response {
	switch request.Operation {
	case "purge":
		return service.handlePurge(ctx, request)
	case "review":
		return service.handleReview(request.Params)
	default:
		return ErrorResponse(ErrorUnknownOperation, "unknown operation")
	}
}

func (service *Service) handlePurge(ctx context.Context, request Request) Response {
	var params PurgeParams
	if err := decodeOperationParams(request.Params, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode purge params")
	}

	result, err := service.journal.Purge(ctx, params)
	if err != nil {
		return entryErrorResponse(err)
	}

	return SuccessResponse(result)
}

func (service *Service) handleRecord(ctx context.Context, encoded json.RawMessage) Response {
	var params RecordEntryParams
	if err := decodeOperationParams(encoded, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode record params")
	}

	result, err := service.workflows.Record(ctx, params)
	if err != nil {
		return entryErrorResponse(err)
	}

	return SuccessResponse(result)
}

func (service *Service) handleRevise(ctx context.Context, encoded json.RawMessage) Response {
	var params ReviseEntryParams
	if err := decodeOperationParams(encoded, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode revise params")
	}

	result, err := service.workflows.Revise(ctx, params)
	if err != nil {
		return entryErrorResponse(err)
	}

	return SuccessResponse(result)
}

func (service *Service) handleLifecycle(ctx context.Context, encoded json.RawMessage, status EntryStatus) Response {
	var params LifecycleEntryParams
	if err := decodeOperationParams(encoded, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode lifecycle params")
	}

	result, err := service.workflows.ChangeStatus(ctx, params, status)
	if err != nil {
		return entryErrorResponse(err)
	}

	return SuccessResponse(result)
}

func (service *Service) handleSearch(encoded json.RawMessage) Response {
	var params SearchEntriesParams
	if err := decodeOperationParams(encoded, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode search params")
	}

	result, err := service.journal.SearchEntries(params)
	if err != nil {
		return entryErrorResponse(err)
	}

	return SuccessResponse(result)
}

func (service *Service) handleReview(encoded json.RawMessage) Response {
	var params ReviewParams
	if err := decodeOperationParams(encoded, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode review params")
	}

	result, err := service.journal.Review(params)
	if err != nil {
		return entryErrorResponse(err)
	}

	return SuccessResponse(result)
}

func (service *Service) handleShow(encoded json.RawMessage) Response {
	var params ShowEntryParams
	if err := decodeOperationParams(encoded, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode show params")
	}

	result, err := service.journal.ShowEntry(params)
	if err != nil {
		return entryErrorResponse(err)
	}

	return SuccessResponse(result)
}

func (service *Service) handleVerify(encoded json.RawMessage) Response {
	var params VerifyEntriesParams
	if err := decodeOperationParams(encoded, &params); err != nil {
		return ErrorResponse(ErrorInvalidRequest, "decode verify params")
	}

	return SuccessResponse(VerifyEntriesResult{
		RevisionCount:  service.journal.Count(),
		EffectiveCount: len(service.journal.EffectiveEntries()),
	})
}

func decodeOperationParams(encoded json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return err
	}

	return requireJSONEnd(decoder)
}

func entryErrorResponse(err error) Response {
	switch {
	case errors.Is(err, ErrEntryNotFound):
		return ErrorResponse(ErrorNotFound, err.Error())
	case errors.Is(err, ErrEntryTargetStale), errors.Is(err, ErrEntryRevisionTargetMissing),
		errors.Is(err, ErrEntryRevisionTargetStale):
		return ErrorResponse(ErrorInvalidEntry, err.Error())
	case errors.Is(err, ErrEntryJournalCorrupt):
		return ErrorResponse(ErrorJournalCorrupt, err.Error())
	case errors.Is(err, ErrEntryJournalNotLoaded):
		return ErrorResponse(ErrorJournalUnavailable, "journal is unavailable")
	default:
		return ErrorResponse(ErrorInvalidRequest, err.Error())
	}
}

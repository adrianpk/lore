package lore

import "time"

// EntryID is the immutable ULID identity of one knowledge revision.
type EntryID string

// EntryKind identifies the semantic purpose of a knowledge entry.
type EntryKind string

const (
	// EntryKindProcedure records repeatable operational steps.
	EntryKindProcedure EntryKind = "procedure"
	// EntryKindFact records a durable operational truth.
	EntryKindFact EntryKind = "fact"
	// EntryKindWarning records a reusable operational hazard.
	EntryKindWarning EntryKind = "warning"
	// EntryKindReference describes a resource and when to consult it.
	EntryKindReference EntryKind = "reference"
)

// EntryStatus identifies the effective lifecycle state of an entry.
type EntryStatus string

const (
	// EntryStatusActive includes an entry in normal retrieval.
	EntryStatusActive EntryStatus = "active"
	// EntryStatusArchived retains knowledge outside normal retrieval.
	EntryStatusArchived EntryStatus = "archived"
	// EntryStatusInvalidated marks retained knowledge as no longer correct.
	EntryStatusInvalidated EntryStatus = "invalidated"
)

// ContextType identifies one structured applicability dimension.
type ContextType string

const (
	// ContextTypeProject identifies a project context.
	ContextTypeProject ContextType = "project"
	// ContextTypeRepository identifies a source repository context.
	ContextTypeRepository ContextType = "repository"
	// ContextTypeSystem identifies an operated technical system.
	ContextTypeSystem ContextType = "system"
	// ContextTypeHost identifies a machine context.
	ContextTypeHost ContextType = "host"
	// ContextTypeTechnology identifies a language, framework, or tool context.
	ContextTypeTechnology ContextType = "technology"
	// ContextTypeClient identifies a client context.
	ContextTypeClient ContextType = "client"
)

// EntryReferenceType identifies the form of a supporting reference.
type EntryReferenceType string

const (
	// EntryReferenceTypeURL identifies a web resource.
	EntryReferenceTypeURL EntryReferenceType = "url"
	// EntryReferenceTypeFile identifies a local or repository-relative file.
	EntryReferenceTypeFile EntryReferenceType = "file"
	// EntryReferenceTypeRepository identifies a source repository.
	EntryReferenceTypeRepository EntryReferenceType = "repository"
	// EntryReferenceTypeCommit identifies a Git commit.
	EntryReferenceTypeCommit EntryReferenceType = "commit"
)

// ProvenanceType identifies the semantic source of a knowledge entry.
type ProvenanceType string

const (
	// ProvenanceTypeHuman identifies human-supplied content.
	ProvenanceTypeHuman ProvenanceType = "human"
	// ProvenanceTypeAgent identifies agent-supplied content.
	ProvenanceTypeAgent ProvenanceType = "agent"
	// ProvenanceTypeIntegration identifies integration-supplied content.
	ProvenanceTypeIntegration ProvenanceType = "integration"
)

// Entry is one complete canonical knowledge revision.
type Entry struct {
	ID             EntryID          `json:"id"`
	Kind           EntryKind        `json:"kind"`
	Status         EntryStatus      `json:"status"`
	Title          string           `json:"title"`
	Body           string           `json:"body"`
	RecordedAt     time.Time        `json:"recorded_at"`
	VerifiedAt     *time.Time       `json:"verified_at,omitempty"`
	Contexts       []Context        `json:"contexts,omitempty"`
	Labels         []string         `json:"labels,omitempty"`
	References     []EntryReference `json:"references,omitempty"`
	Provenance     *Provenance      `json:"provenance,omitempty"`
	Supersedes     EntryID          `json:"supersedes,omitempty"`
	RevisionReason string           `json:"revision_reason,omitempty"`
}

// Context identifies one structured applicability dimension for an entry.
type Context struct {
	Type       ContextType `json:"type"`
	Name       string      `json:"name"`
	Repository string      `json:"repository,omitempty"`
}

// EntryReference identifies one resource supporting an entry.
type EntryReference struct {
	Type  EntryReferenceType `json:"type"`
	Value string             `json:"value"`
	Label string             `json:"label,omitempty"`
}

// Provenance describes the semantic origin of captured knowledge.
type Provenance struct {
	Type           ProvenanceType `json:"type"`
	Harness        string         `json:"harness,omitempty"`
	HarnessVersion string         `json:"harness_version,omitempty"`
	Model          string         `json:"model,omitempty"`
}

// EntryProjection is the compact search and review representation.
type EntryProjection struct {
	ID         EntryID     `json:"id"`
	Kind       EntryKind   `json:"kind"`
	Status     EntryStatus `json:"status"`
	Title      string      `json:"title"`
	RecordedAt time.Time   `json:"recorded_at"`
	Contexts   []Context   `json:"contexts,omitempty"`
}

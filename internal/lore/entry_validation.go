package lore

import (
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	maxEntryTitleRunes      = 200
	maxEntryBodyBytes       = 32 * 1024
	maxEntryContexts        = 16
	maxEntryLabels          = 8
	maxEntryReferences      = 32
	maxCanonicalNameBytes   = 64
	maxRepositoryBytes      = 512
	maxReferenceValueBytes  = 2048
	maxReferenceLabelRunes  = 64
	maxProvenanceFieldRunes = 64
	maxRevisionReasonRunes  = 500
)

var sensitiveQueryKeys = map[string]struct{}{
	"access_token":  {},
	"api_key":       {},
	"apikey":        {},
	"auth":          {},
	"authorization": {},
	"credential":    {},
	"key":           {},
	"password":      {},
	"secret":        {},
	"sig":           {},
	"signature":     {},
	"token":         {},
}

// Canonicalize returns a normalized copy of the entry and validates it.
func (entry Entry) Canonicalize() (Entry, error) {
	entry.Title = strings.TrimSpace(entry.Title)
	entry.Body = strings.TrimSpace(entry.Body)
	entry.RevisionReason = strings.TrimSpace(entry.RevisionReason)

	contexts, err := canonicalizeContexts(entry.Contexts)
	if err != nil {
		return Entry{}, err
	}

	entry.Contexts = contexts

	labels, err := canonicalizeLabels(entry.Labels)
	if err != nil {
		return Entry{}, err
	}

	entry.Labels = labels
	entry.References = canonicalizeReferences(entry.References)
	entry.Provenance = canonicalizeProvenance(entry.Provenance)

	if err := entry.Validate(); err != nil {
		return Entry{}, err
	}

	return entry, nil
}

// Validate checks the complete canonical entry contract.
func (entry Entry) Validate() error {
	if err := entry.validateIdentityAndContent(); err != nil {
		return err
	}

	if err := validateEntryContexts(entry.Contexts); err != nil {
		return err
	}

	if err := validateEntryLabels(entry.Labels); err != nil {
		return err
	}

	if err := validateEntryReferences(entry.References); err != nil {
		return err
	}

	if entry.Provenance != nil {
		if err := entry.Provenance.Validate(); err != nil {
			return err
		}
	}

	return entry.validateRevision()
}

// NewEntryRevision builds and validates a complete successor revision.
func NewEntryRevision(previous, replacement Entry, id EntryID, recordedAt time.Time, reason string) (Entry, error) {
	if err := previous.Validate(); err != nil {
		return Entry{}, fmt.Errorf("revise entry: validate predecessor: %w", err)
	}

	if recordedAt.Before(previous.RecordedAt) {
		return Entry{}, fmt.Errorf("revise entry: recorded_at must not precede predecessor")
	}

	replacement.ID = id
	replacement.RecordedAt = recordedAt
	replacement.Supersedes = previous.ID
	replacement.RevisionReason = reason

	canonical, err := replacement.Canonicalize()
	if err != nil {
		return Entry{}, fmt.Errorf("revise entry: %w", err)
	}

	if canonical.Kind != previous.Kind {
		return Entry{}, fmt.Errorf("revise entry: kind must remain %q", previous.Kind)
	}

	return canonical, nil
}

// NormalizeCanonicalName converts a context name or label to lowercase kebab
// form.
func NormalizeCanonicalName(value string) (string, error) {
	var normalized strings.Builder

	separator := false

	for _, character := range strings.TrimSpace(value) {
		if isASCIIAlphaNumeric(character) {
			if separator && normalized.Len() > 0 {
				normalized.WriteByte('-')
			}

			normalized.WriteRune(unicode.ToLower(character))

			separator = false

			continue
		}

		separator = normalized.Len() > 0
	}

	result := normalized.String()
	if result == "" {
		return "", fmt.Errorf("normalize canonical name: value is empty")
	}

	if len(result) > maxCanonicalNameBytes {
		return "", fmt.Errorf("normalize canonical name: value exceeds %d bytes", maxCanonicalNameBytes)
	}

	return result, nil
}

// Validate checks one canonical entry context.
func (context Context) Validate() error {
	if !context.Type.Valid() {
		return fmt.Errorf("validate context: unsupported type %q", context.Type)
	}

	if err := validateCanonicalName("context name", context.Name); err != nil {
		return err
	}

	return validateRepositoryIdentity(context.Repository)
}

// Validate checks one supporting entry reference.
func (reference EntryReference) Validate() error {
	if !reference.Type.Valid() {
		return fmt.Errorf("validate entry reference: unsupported type %q", reference.Type)
	}

	if err := validateBoundedSingleLine("entry reference value", reference.Value, true, maxReferenceValueBytes, 0); err != nil {
		return err
	}

	if err := validateBoundedSingleLine("entry reference label", reference.Label, false, 0, maxReferenceLabelRunes); err != nil {
		return err
	}

	if reference.Type == EntryReferenceTypeURL {
		return validateEntryURL(reference.Value)
	}

	if reference.Type == EntryReferenceTypeRepository {
		return rejectLocatorCredentials("entry reference value", reference.Value)
	}

	return nil
}

// Validate checks optional entry provenance.
func (provenance Provenance) Validate() error {
	if !provenance.Type.Valid() {
		return fmt.Errorf("validate entry provenance: unsupported type %q", provenance.Type)
	}

	fields := []struct {
		name  string
		value string
	}{
		{name: "harness", value: provenance.Harness},
		{name: "harness_version", value: provenance.HarnessVersion},
		{name: "model", value: provenance.Model},
	}

	for _, field := range fields {
		if err := validateBoundedSingleLine("entry provenance "+field.name, field.value, false, 0, maxProvenanceFieldRunes); err != nil {
			return err
		}
	}

	return nil
}

// Valid reports whether the entry kind is canonical.
func (kind EntryKind) Valid() bool {
	switch kind {
	case EntryKindProcedure, EntryKindFact, EntryKindWarning, EntryKindReference:
		return true
	default:
		return false
	}
}

// Valid reports whether the entry status is canonical.
func (status EntryStatus) Valid() bool {
	switch status {
	case EntryStatusActive, EntryStatusArchived, EntryStatusInvalidated:
		return true
	default:
		return false
	}
}

// Valid reports whether the context type is canonical.
func (contextType ContextType) Valid() bool {
	switch contextType {
	case ContextTypeProject, ContextTypeRepository, ContextTypeSystem, ContextTypeHost,
		ContextTypeTechnology, ContextTypeClient:
		return true
	default:
		return false
	}
}

// Valid reports whether the reference type is canonical.
func (referenceType EntryReferenceType) Valid() bool {
	switch referenceType {
	case EntryReferenceTypeURL, EntryReferenceTypeFile, EntryReferenceTypeRepository,
		EntryReferenceTypeCommit:
		return true
	default:
		return false
	}
}

// Valid reports whether the provenance type is canonical.
func (provenanceType ProvenanceType) Valid() bool {
	switch provenanceType {
	case ProvenanceTypeHuman, ProvenanceTypeAgent, ProvenanceTypeIntegration:
		return true
	default:
		return false
	}
}

// Validate checks the canonical ULID representation of an entry ID.
func (id EntryID) Validate() error {
	value := string(id)
	if len(value) != 26 {
		return fmt.Errorf("validate entry ID: ULID must contain 26 characters")
	}

	if value[0] > '7' {
		return fmt.Errorf("validate entry ID: ULID exceeds 128 bits")
	}

	for _, character := range value {
		if !strings.ContainsRune(crockfordBase32, character) {
			return fmt.Errorf("validate entry ID: ULID contains invalid character %q", character)
		}
	}

	return nil
}

func (entry Entry) validateIdentityAndContent() error {
	if err := entry.ID.Validate(); err != nil {
		return err
	}

	if !entry.Kind.Valid() {
		return fmt.Errorf("validate entry: unsupported kind %q", entry.Kind)
	}

	if !entry.Status.Valid() {
		return fmt.Errorf("validate entry: unsupported status %q", entry.Status)
	}

	if err := validateBoundedSingleLine("entry title", entry.Title, true, 0, maxEntryTitleRunes); err != nil {
		return err
	}

	if err := validateEntryBody(entry.Body); err != nil {
		return err
	}

	if err := validateUTCTime("entry recorded_at", entry.RecordedAt); err != nil {
		return err
	}

	if entry.VerifiedAt == nil {
		return nil
	}

	if err := validateUTCTime("entry verified_at", *entry.VerifiedAt); err != nil {
		return err
	}

	if entry.VerifiedAt.After(entry.RecordedAt) {
		return fmt.Errorf("validate entry: verified_at must not follow recorded_at")
	}

	return nil
}

func (entry Entry) validateRevision() error {
	if entry.Supersedes == "" {
		if entry.RevisionReason != "" {
			return fmt.Errorf("validate entry: revision_reason requires supersedes")
		}

		return nil
	}

	if err := entry.Supersedes.Validate(); err != nil {
		return fmt.Errorf("validate entry supersedes: %w", err)
	}

	if entry.Supersedes == entry.ID {
		return fmt.Errorf("validate entry: entry cannot supersede itself")
	}

	return validateBoundedSingleLine("entry revision_reason", entry.RevisionReason, true, 0, maxRevisionReasonRunes)
}

func canonicalizeContexts(contexts []Context) ([]Context, error) {
	canonical := make([]Context, len(contexts))
	for index, context := range contexts {
		name, err := NormalizeCanonicalName(context.Name)
		if err != nil {
			return nil, fmt.Errorf("canonicalize context %d: %w", index, err)
		}

		context.Name = name
		context.Repository = strings.TrimSpace(context.Repository)
		canonical[index] = context
	}

	return canonical, nil
}

func canonicalizeLabels(labels []string) ([]string, error) {
	canonical := make([]string, len(labels))
	for index, label := range labels {
		normalized, err := NormalizeCanonicalName(label)
		if err != nil {
			return nil, fmt.Errorf("canonicalize label %d: %w", index, err)
		}

		canonical[index] = normalized
	}

	return canonical, nil
}

func canonicalizeReferences(references []EntryReference) []EntryReference {
	canonical := make([]EntryReference, len(references))
	for index, reference := range references {
		reference.Value = strings.TrimSpace(reference.Value)
		reference.Label = strings.TrimSpace(reference.Label)
		canonical[index] = reference
	}

	return canonical
}

func canonicalizeProvenance(provenance *Provenance) *Provenance {
	if provenance == nil {
		return nil
	}

	canonical := *provenance
	canonical.Harness = strings.TrimSpace(canonical.Harness)
	canonical.HarnessVersion = strings.TrimSpace(canonical.HarnessVersion)
	canonical.Model = strings.TrimSpace(canonical.Model)

	return &canonical
}

func validateEntryContexts(contexts []Context) error {
	if len(contexts) > maxEntryContexts {
		return fmt.Errorf("validate entry: contexts exceed %d items", maxEntryContexts)
	}

	seen := make(map[string]struct{}, len(contexts))
	for _, context := range contexts {
		if err := context.Validate(); err != nil {
			return err
		}

		key := strings.Join([]string{string(context.Type), context.Name, context.Repository}, "\x00")
		if _, exists := seen[key]; exists {
			return fmt.Errorf("validate entry: duplicate context")
		}

		seen[key] = struct{}{}
	}

	return nil
}

func validateEntryLabels(labels []string) error {
	if len(labels) > maxEntryLabels {
		return fmt.Errorf("validate entry: labels exceed %d items", maxEntryLabels)
	}

	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		if err := validateCanonicalName("entry label", label); err != nil {
			return err
		}

		if _, exists := seen[label]; exists {
			return fmt.Errorf("validate entry: duplicate label")
		}

		seen[label] = struct{}{}
	}

	return nil
}

func validateEntryReferences(references []EntryReference) error {
	if len(references) > maxEntryReferences {
		return fmt.Errorf("validate entry: references exceed %d items", maxEntryReferences)
	}

	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if err := reference.Validate(); err != nil {
			return err
		}

		key := string(reference.Type) + "\x00" + reference.Value
		if _, exists := seen[key]; exists {
			return fmt.Errorf("validate entry: duplicate reference")
		}

		seen[key] = struct{}{}
	}

	return nil
}

func validateCanonicalName(name, value string) error {
	canonical, err := NormalizeCanonicalName(value)
	if err != nil {
		return fmt.Errorf("validate %s: %w", name, err)
	}

	if canonical != value {
		return fmt.Errorf("validate %s: value must use canonical lowercase kebab form", name)
	}

	return nil
}

func validateRepositoryIdentity(value string) error {
	if err := validateBoundedSingleLine("context repository", value, false, maxRepositoryBytes, 0); err != nil {
		return err
	}

	return rejectLocatorCredentials("context repository", value)
}

func validateEntryURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("validate entry reference URL: %w", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("validate entry reference URL: must use HTTP or HTTPS")
	}

	if parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("validate entry reference URL: must have a host and no user information")
	}

	for key := range parsed.Query() {
		if _, sensitive := sensitiveQueryKeys[strings.ToLower(key)]; sensitive {
			return fmt.Errorf("validate entry reference URL: query contains sensitive key %q", key)
		}
	}

	return nil
}

func rejectLocatorCredentials(name, value string) error {
	if value == "" {
		return nil
	}

	parsed, err := url.Parse(value)
	if err == nil && parsed.User != nil {
		return fmt.Errorf("validate %s: user information is not allowed", name)
	}

	if strings.Contains(value, "@") {
		return fmt.Errorf("validate %s: credential-like user information is not allowed", name)
	}

	return nil
}

func validateEntryBody(value string) error {
	if value == "" {
		return fmt.Errorf("validate entry body: value is required")
	}

	if !utf8.ValidString(value) {
		return fmt.Errorf("validate entry body: value must be valid UTF-8")
	}

	if strings.TrimSpace(value) != value {
		return fmt.Errorf("validate entry body: value must be trimmed")
	}

	if len(value) > maxEntryBodyBytes {
		return fmt.Errorf("validate entry body: value exceeds %d bytes", maxEntryBodyBytes)
	}

	for _, character := range value {
		if unicode.IsControl(character) && character != '\n' && character != '\t' {
			return fmt.Errorf("validate entry body: value contains a control character")
		}
	}

	return nil
}

func validateUTCTime(name string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("validate entry: %s is required", name)
	}

	_, offset := value.Zone()
	if offset != 0 {
		return fmt.Errorf("validate entry: %s must use UTC", name)
	}

	return nil
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}

	return false
}

func validateBoundedSingleLine(name, value string, required bool, maxBytes, maxRunes int) error {
	if value == "" {
		if required {
			return fmt.Errorf("validate %s: value is required", name)
		}

		return nil
	}

	if err := validateSingleLineText(name, value); err != nil {
		return err
	}

	if maxBytes > 0 && len(value) > maxBytes {
		return fmt.Errorf("validate %s: value exceeds %d bytes", name, maxBytes)
	}

	if maxRunes > 0 && utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("validate %s: value exceeds %d Unicode code points", name, maxRunes)
	}

	return nil
}

func validateSingleLineText(name, value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("validate %s: value must be valid UTF-8", name)
	}

	if strings.TrimSpace(value) != value {
		return fmt.Errorf("validate %s: value must be trimmed", name)
	}

	if strings.ContainsAny(value, "\r\n") || containsControl(value) {
		return fmt.Errorf("validate %s: value must be one line without control characters", name)
	}

	return nil
}

func isASCIIAlphaNumeric(character rune) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9'
}

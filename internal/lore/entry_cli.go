package lore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const (
	recordEntryUsage = "usage: lore record <procedure|fact|warning|reference> --title <title> (--body <body> | --body-file <path>) [flags]"
	reviseEntryUsage = "usage: lore revise --target <id> --reason <reason> [replacement flags]"
)

type recordEntryOptions struct {
	params     RecordEntryParams
	bodyFile   string
	jsonInput  bool
	jsonOutput bool
	contexts   entryStringListFlag
	labels     entryStringListFlag
	references entryStringListFlag
	verifiedAt string
	source     string
	harness    string
	version    string
	model      string
}

type reviseEntryOptions struct {
	params          ReviseEntryParams
	bodyFile        string
	jsonInput       bool
	jsonOutput      bool
	contexts        entryStringListFlag
	labels          entryStringListFlag
	references      entryStringListFlag
	verifiedAt      string
	clearContexts   bool
	clearLabels     bool
	clearReferences bool
}

type entryStringListFlag []string

func (flagValue *entryStringListFlag) String() string {
	return strings.Join(*flagValue, ",")
}

func (flagValue *entryStringListFlag) Set(value string) error {
	*flagValue = append(*flagValue, value)

	return nil
}

func (app *App) runEntryRecord(ctx context.Context, arguments []string) error {
	options, err := parseRecordEntryOptions(arguments, app.stdin, app.stderr)
	if err != nil {
		return err
	}

	if !hasProjectContext(options.params.Contexts) {
		inferred, ok := app.inferProjectContext(ctx)
		if ok {
			options.params.Contexts = append(options.params.Contexts, inferred)
		}
	}

	var result EntryMutationResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "record", options.params, &result); err != nil {
		return err
	}

	return writeEntryMutation(app.stdout, result, options.jsonOutput)
}

func parseRecordEntryOptions(arguments []string, input io.Reader, errorOutput io.Writer) (recordEntryOptions, error) {
	if len(arguments) == 0 {
		return recordEntryOptions{}, errors.New(recordEntryUsage)
	}

	kind := EntryKind(arguments[0])
	if !kind.Valid() {
		return recordEntryOptions{}, fmt.Errorf("%s: unsupported kind %q", recordEntryUsage, kind)
	}

	options := recordEntryOptions{params: RecordEntryParams{Kind: kind}}
	flags := flag.NewFlagSet("record "+arguments[0], flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	bindRecordEntryFlags(flags, &options)

	if err := flags.Parse(arguments[1:]); err != nil {
		return recordEntryOptions{}, err
	}

	if len(flags.Args()) != 0 {
		return recordEntryOptions{}, errors.New(recordEntryUsage)
	}

	if options.jsonInput {
		return parseRecordEntryJSON(input, kind, options.jsonOutput)
	}

	if err := applyRecordEntryFlags(&options); err != nil {
		return recordEntryOptions{}, err
	}

	return options, nil
}

func bindRecordEntryFlags(flags *flag.FlagSet, options *recordEntryOptions) {
	flags.StringVar(&options.params.Title, "title", "", "entry title")
	flags.StringVar(&options.params.Body, "body", "", "Markdown body")
	flags.StringVar(&options.bodyFile, "body-file", "", "read the Markdown body from a file")
	flags.StringVar(&options.verifiedAt, "verified-at", "", "RFC3339 verification time")
	flags.Var(&options.contexts, "context", "type:name or type:name@repository")
	flags.Var(&options.labels, "label", "topic label")
	flags.Var(&options.references, "reference", "type:value")
	flags.StringVar(&options.source, "source", "", "human, agent, or integration")
	flags.StringVar(&options.harness, "harness", "", "agent harness name")
	flags.StringVar(&options.version, "harness-version", "", "agent harness version")
	flags.StringVar(&options.model, "model", "", "model identifier")
	flags.BoolVar(&options.jsonInput, "json-input", false, "read entry fields as JSON from stdin")
	flags.BoolVar(&options.jsonOutput, "json", false, "write JSON output")
}

func applyRecordEntryFlags(options *recordEntryOptions) error {
	body, err := readExclusiveBody(options.params.Body, options.bodyFile)
	if err != nil {
		return err
	}

	verifiedAt, err := parseEntryTime("verified-at", options.verifiedAt)
	if err != nil {
		return err
	}

	contexts, err := parseEntryContexts(options.contexts)
	if err != nil {
		return err
	}

	references, err := parseEntryReferences(options.references)
	if err != nil {
		return err
	}

	provenance, err := parseEntryProvenance(options.source, options.harness, options.version, options.model)
	if err != nil {
		return err
	}

	options.params.Body = body
	options.params.VerifiedAt = verifiedAt
	options.params.Contexts = contexts
	options.params.Labels = append([]string(nil), options.labels...)
	options.params.References = references
	options.params.Provenance = provenance

	if strings.TrimSpace(options.params.Title) == "" {
		return errors.New(recordEntryUsage)
	}

	return nil
}

func parseRecordEntryJSON(input io.Reader, kind EntryKind, jsonOutput bool) (recordEntryOptions, error) {
	var params RecordEntryParams
	if err := decodeCLIJSON(input, &params); err != nil {
		return recordEntryOptions{}, err
	}

	if params.Kind != "" && params.Kind != kind {
		return recordEntryOptions{}, fmt.Errorf("record entry: JSON kind must match positional kind %q", kind)
	}

	params.Kind = kind

	return recordEntryOptions{params: params, jsonInput: true, jsonOutput: jsonOutput}, nil
}

func (app *App) runEntryRevise(ctx context.Context, arguments []string) error {
	options, err := parseReviseEntryOptions(arguments, app.stdin, app.stderr)
	if err != nil {
		return err
	}

	var result EntryMutationResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "revise", options.params, &result); err != nil {
		return err
	}

	return writeEntryMutation(app.stdout, result, options.jsonOutput)
}

func parseReviseEntryOptions(arguments []string, input io.Reader, errorOutput io.Writer) (reviseEntryOptions, error) {
	options := reviseEntryOptions{}
	flags := flag.NewFlagSet("revise", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	bindReviseEntryFlags(flags, &options)

	if err := flags.Parse(arguments); err != nil {
		return reviseEntryOptions{}, err
	}

	if len(flags.Args()) != 0 {
		return reviseEntryOptions{}, errors.New(reviseEntryUsage)
	}

	if options.jsonInput {
		return parseReviseEntryJSON(input, options.jsonOutput)
	}

	if err := applyReviseEntryFlags(&options); err != nil {
		return reviseEntryOptions{}, err
	}

	return options, nil
}

func bindReviseEntryFlags(flags *flag.FlagSet, options *reviseEntryOptions) {
	flags.Var(optionalEntryString{target: &options.params.Title}, "title", "replacement title")
	flags.Var(optionalEntryString{target: &options.params.Body}, "body", "replacement Markdown body")
	flags.StringVar(&options.bodyFile, "body-file", "", "read replacement body from a file")
	flags.StringVar(&options.verifiedAt, "verified-at", "", "replacement RFC3339 verification time")
	flags.BoolVar(&options.params.ClearVerifiedAt, "clear-verified-at", false, "remove verification time")
	flags.Var(&options.contexts, "context", "replacement type:name or type:name@repository")
	flags.BoolVar(&options.clearContexts, "clear-contexts", false, "remove all contexts")
	flags.Var(&options.labels, "label", "replacement topic label")
	flags.BoolVar(&options.clearLabels, "clear-labels", false, "remove all labels")
	flags.Var(&options.references, "reference", "replacement type:value")
	flags.BoolVar(&options.clearReferences, "clear-references", false, "remove all references")
	flags.StringVar((*string)(&options.params.Target), "target", "", "effective entry ID")
	flags.StringVar(&options.params.Reason, "reason", "", "revision reason")
	flags.BoolVar(&options.jsonInput, "json-input", false, "read revision fields as JSON from stdin")
	flags.BoolVar(&options.jsonOutput, "json", false, "write JSON output")
}

func applyReviseEntryFlags(options *reviseEntryOptions) error {
	if options.params.Target == "" || strings.TrimSpace(options.params.Reason) == "" {
		return errors.New(reviseEntryUsage)
	}

	if options.bodyFile != "" {
		if options.params.Body != nil {
			return fmt.Errorf("revise entry: --body and --body-file are mutually exclusive")
		}

		body, err := os.ReadFile(options.bodyFile)
		if err != nil {
			return fmt.Errorf("read entry body file: %w", err)
		}

		value := string(body)
		options.params.Body = &value
	}

	if err := applyReviseEntryCollections(options); err != nil {
		return err
	}

	verifiedAt, err := parseEntryTime("verified-at", options.verifiedAt)
	if err != nil {
		return err
	}

	if verifiedAt != nil && options.params.ClearVerifiedAt {
		return fmt.Errorf("revise entry: --verified-at and --clear-verified-at are mutually exclusive")
	}

	options.params.VerifiedAt = verifiedAt

	return nil
}

func applyReviseEntryCollections(options *reviseEntryOptions) error {
	contexts, err := parseEntryContexts(options.contexts)
	if err != nil {
		return err
	}

	references, err := parseEntryReferences(options.references)
	if err != nil {
		return err
	}

	if err := setReplacementCollection("contexts", len(contexts), options.clearContexts, &options.params.Contexts, contexts); err != nil {
		return err
	}

	labels := []string(options.labels)
	if err := setReplacementCollection("labels", len(labels), options.clearLabels, &options.params.Labels, labels); err != nil {
		return err
	}

	return setReplacementCollection(
		"references",
		len(references),
		options.clearReferences,
		&options.params.References,
		references,
	)
}

func setReplacementCollection[T any](name string, count int, clear bool, target **[]T, values []T) error {
	if count > 0 && clear {
		return fmt.Errorf("revise entry: --%s and --clear-%s are mutually exclusive", strings.TrimSuffix(name, "s"), name)
	}

	if count > 0 {
		*target = &values
	} else if clear {
		empty := make([]T, 0)
		*target = &empty
	}

	return nil
}

func parseReviseEntryJSON(input io.Reader, jsonOutput bool) (reviseEntryOptions, error) {
	var params ReviseEntryParams
	if err := decodeCLIJSON(input, &params); err != nil {
		return reviseEntryOptions{}, err
	}

	if params.Target == "" || strings.TrimSpace(params.Reason) == "" {
		return reviseEntryOptions{}, errors.New(reviseEntryUsage)
	}

	return reviseEntryOptions{params: params, jsonInput: true, jsonOutput: jsonOutput}, nil
}

type optionalEntryString struct {
	target **string
}

func (value optionalEntryString) String() string {
	if value.target == nil || *value.target == nil {
		return ""
	}

	return **value.target
}

func (value optionalEntryString) Set(input string) error {
	*value.target = &input

	return nil
}

func (app *App) runEntryLifecycle(ctx context.Context, operation string, arguments []string) error {
	params, jsonOutput, err := parseLifecycleEntryOptions(operation, arguments, app.stderr)
	if err != nil {
		return err
	}

	var result EntryMutationResult
	if err := NewClient(app.config.SocketPath).Call(ctx, operation, params, &result); err != nil {
		return err
	}

	return writeEntryMutation(app.stdout, result, jsonOutput)
}

func parseLifecycleEntryOptions(operation string, arguments []string, errorOutput io.Writer) (LifecycleEntryParams, bool, error) {
	flags := flag.NewFlagSet(operation, flag.ContinueOnError)
	flags.SetOutput(errorOutput)

	var (
		params     LifecycleEntryParams
		jsonOutput bool
	)

	flags.StringVar((*string)(&params.Target), "target", "", "effective entry ID")
	flags.StringVar(&params.Reason, "reason", "", "lifecycle revision reason")
	flags.BoolVar(&jsonOutput, "json", false, "write JSON output")

	if err := flags.Parse(arguments); err != nil {
		return LifecycleEntryParams{}, false, err
	}

	if len(flags.Args()) != 0 || params.Target == "" || strings.TrimSpace(params.Reason) == "" {
		return LifecycleEntryParams{}, false, fmt.Errorf("usage: lore %s --target <id> --reason <reason> [--json]", operation)
	}

	return params, jsonOutput, nil
}

func readExclusiveBody(body, path string) (string, error) {
	if (body == "") == (path == "") {
		return "", fmt.Errorf("record entry: exactly one of --body or --body-file is required")
	}

	if path == "" {
		return body, nil
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read entry body file: %w", err)
	}

	return string(contents), nil
}

func parseEntryTime(name, value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, fmt.Errorf("parse --%s: %w", name, err)
	}

	utc := parsed.UTC()

	return &utc, nil
}

func parseEntryContexts(values []string) ([]Context, error) {
	contexts := make([]Context, 0, len(values))

	for _, value := range values {
		contextValue, err := parseEntryContext(value)
		if err != nil {
			return nil, err
		}

		contexts = append(contexts, contextValue)
	}

	return contexts, nil
}

func parseEntryContext(value string) (Context, error) {
	typeValue, remainder, found := strings.Cut(value, ":")
	if !found || typeValue == "" || remainder == "" {
		return Context{}, fmt.Errorf("parse --context: expected type:name or type:name@repository")
	}

	name, repository, _ := strings.Cut(remainder, "@")
	contextValue := Context{
		Type:       ContextType(strings.ToLower(strings.TrimSpace(typeValue))),
		Name:       name,
		Repository: repository,
	}

	canonical, err := canonicalizeContexts([]Context{contextValue})
	if err != nil {
		return Context{}, err
	}

	if err := canonical[0].Validate(); err != nil {
		return Context{}, err
	}

	return canonical[0], nil
}

func parseEntryReferences(values []string) ([]EntryReference, error) {
	references := make([]EntryReference, 0, len(values))

	for _, value := range values {
		typeValue, referenceValue, found := strings.Cut(value, ":")
		if !found || typeValue == "" || referenceValue == "" {
			return nil, fmt.Errorf("parse --reference: expected type:value")
		}

		references = append(references, EntryReference{Type: EntryReferenceType(typeValue), Value: referenceValue})
	}

	return references, nil
}

func parseEntryProvenance(source, harness, version, model string) (*Provenance, error) {
	if source == "" && harness == "" && version == "" && model == "" {
		return nil, nil
	}

	provenance := &Provenance{
		Type:           ProvenanceType(source),
		Harness:        harness,
		HarnessVersion: version,
		Model:          model,
	}

	if err := provenance.Validate(); err != nil {
		return nil, err
	}

	return provenance, nil
}

func decodeCLIJSON(input io.Reader, target any) error {
	encoded, err := io.ReadAll(io.LimitReader(input, MaxRequestSize+1))
	if err != nil {
		return fmt.Errorf("read JSON input: %w", err)
	}

	if len(encoded) > MaxRequestSize {
		return fmt.Errorf("read JSON input: input exceeds the allowed size")
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode JSON input: %w", err)
	}

	if err := requireJSONEnd(decoder); err != nil {
		return fmt.Errorf("decode JSON input: %w", err)
	}

	return nil
}

func hasProjectContext(contexts []Context) bool {
	for _, contextValue := range contexts {
		if contextValue.Type == ContextTypeProject {
			return true
		}
	}

	return false
}

func writeEntryMutation(writer io.Writer, result EntryMutationResult, jsonOutput bool) error {
	if jsonOutput {
		return writeEntryJSON(writer, result)
	}

	_, err := fmt.Fprintf(writer, "%s %s %s %q\n", result.Entry.ID, result.Entry.Kind, result.Entry.Status, result.Entry.Title)

	return err
}

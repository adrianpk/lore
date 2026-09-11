package lore

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type searchEntriesOptions struct {
	params       SearchEntriesParams
	jsonOutput   bool
	kinds        entryStringListFlag
	statuses     entryStringListFlag
	contexts     entryStringListFlag
	labels       entryStringListFlag
	recordedFrom string
	recordedTo   string
}

func (app *App) runEntrySearch(ctx context.Context, arguments []string) error {
	options, err := parseSearchEntriesOptions(arguments, app.stderr)
	if err != nil {
		return err
	}

	var result SearchEntriesResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "search", options.params, &result); err != nil {
		return err
	}

	if options.jsonOutput {
		return writeEntryJSON(app.stdout, result)
	}

	for _, entry := range result.Entries {
		if err := writeEntryProjection(app.stdout, entry); err != nil {
			return err
		}
	}

	if result.Truncated {
		_, err = fmt.Fprintf(app.stdout, "truncated after %d entries\n", result.ReturnedCount)
	}

	return err
}

func parseSearchEntriesOptions(arguments []string, errorOutput io.Writer) (searchEntriesOptions, error) {
	options := searchEntriesOptions{}
	flags := flag.NewFlagSet("search", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	bindSearchEntriesFlags(flags, &options)

	if err := flags.Parse(arguments); err != nil {
		return searchEntriesOptions{}, err
	}

	if len(flags.Args()) != 0 {
		return searchEntriesOptions{}, fmt.Errorf("usage: lore search [flags]")
	}

	if err := applySearchEntriesFlags(&options); err != nil {
		return searchEntriesOptions{}, err
	}

	return options, nil
}

func bindSearchEntriesFlags(flags *flag.FlagSet, options *searchEntriesOptions) {
	flags.StringVar(&options.params.Text, "text", "", "case-insensitive title and body text")
	flags.Var(&options.kinds, "kind", "entry kind")
	flags.Var(&options.statuses, "status", "entry lifecycle status")
	flags.Var(&options.contexts, "context", "required type:name or type:name@repository")
	flags.Var(&options.labels, "label", "matching topic label")
	flags.StringVar(&options.recordedFrom, "recorded-from", "", "inclusive RFC3339 recording time")
	flags.StringVar(&options.recordedTo, "recorded-to", "", "exclusive RFC3339 recording time")
	flags.StringVar((*string)(&options.params.Order), "order", "", "ascending or descending")
	flags.IntVar(&options.params.Limit, "limit", 0, "maximum returned entries")
	flags.BoolVar(&options.jsonOutput, "json", false, "write JSON output")
}

func applySearchEntriesFlags(options *searchEntriesOptions) error {
	options.params.Kinds = make([]EntryKind, len(options.kinds))
	for index, value := range options.kinds {
		options.params.Kinds[index] = EntryKind(value)
	}

	options.params.Statuses = make([]EntryStatus, len(options.statuses))
	for index, value := range options.statuses {
		options.params.Statuses[index] = EntryStatus(value)
	}

	contexts, err := parseEntryContexts(options.contexts)
	if err != nil {
		return err
	}

	options.params.Contexts = contexts
	options.params.Labels = append([]string(nil), options.labels...)

	options.params.RecordedFrom, err = parseEntryTime("recorded-from", options.recordedFrom)
	if err != nil {
		return err
	}

	options.params.RecordedTo, err = parseEntryTime("recorded-to", options.recordedTo)
	if err != nil {
		return err
	}

	_, err = canonicalSearchParams(options.params)

	return err
}

func (app *App) runEntryShow(ctx context.Context, arguments []string) error {
	params, jsonOutput, err := parseShowEntryOptions(arguments, app.stderr)
	if err != nil {
		return err
	}

	var result ShowEntryResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "show", params, &result); err != nil {
		return err
	}

	if jsonOutput {
		return writeEntryJSON(app.stdout, result)
	}

	if params.History {
		for _, entry := range result.History {
			if err := writeEntryDetail(app.stdout, entry); err != nil {
				return err
			}
		}

		return nil
	}

	return writeEntryDetail(app.stdout, *result.Entry)
}

func parseShowEntryOptions(arguments []string, errorOutput io.Writer) (ShowEntryParams, bool, error) {
	flags := flag.NewFlagSet("show", flag.ContinueOnError)
	flags.SetOutput(errorOutput)

	var (
		params     ShowEntryParams
		jsonOutput bool
	)

	flags.StringVar((*string)(&params.ID), "id", "", "entry ID")
	flags.BoolVar(&params.History, "history", false, "return the complete revision history")
	flags.BoolVar(&jsonOutput, "json", false, "write JSON output")

	if err := flags.Parse(arguments); err != nil {
		return ShowEntryParams{}, false, err
	}

	if len(flags.Args()) != 0 || params.ID == "" {
		return ShowEntryParams{}, false, fmt.Errorf("usage: lore show --id <entry-id> [--history] [--json]")
	}

	return params, jsonOutput, nil
}

func writeEntryJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)

	return encoder.Encode(value)
}

func writeEntryProjection(writer io.Writer, entry Entry) error {
	contextNames := make([]string, len(entry.Contexts))
	for index, contextValue := range entry.Contexts {
		contextNames[index] = string(contextValue.Type) + ":" + contextValue.Name
	}

	_, err := fmt.Fprintf(
		writer,
		"%s %s %s %s %s %q\n",
		entry.RecordedAt.Format(time.RFC3339),
		entry.ID,
		entry.Kind,
		entry.Status,
		strings.Join(contextNames, ","),
		entry.Title,
	)

	return err
}

func writeEntryDetail(writer io.Writer, entry Entry) error {
	_, err := fmt.Fprintf(
		writer,
		"%s %s %s %q\n%s\n",
		entry.ID,
		entry.Kind,
		entry.Status,
		entry.Title,
		entry.Body,
	)

	return err
}

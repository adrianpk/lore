package lore

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
)

type reviewOptions struct {
	params         ReviewParams
	statuses       entryStringListFlag
	contexts       entryStringListFlag
	recordedBefore string
	verifiedBefore string
	jsonOutput     bool
}

func (app *App) runReview(ctx context.Context, arguments []string) error {
	options, err := parseReviewOptions(arguments, app.stderr)
	if err != nil {
		return err
	}

	var result ReviewResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "review", options.params, &result); err != nil {
		return err
	}

	if options.jsonOutput {
		return writeEntryJSON(app.stdout, result)
	}

	for _, candidate := range result.Candidates {
		if err := writeReviewCandidate(app.stdout, candidate); err != nil {
			return err
		}
	}

	if result.Truncated {
		_, err = fmt.Fprintf(app.stdout, "truncated after %d candidates\n", result.ReturnedCount)
	}

	return err
}

func parseReviewOptions(arguments []string, errorOutput io.Writer) (reviewOptions, error) {
	options := reviewOptions{}
	flags := flag.NewFlagSet("review", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	bindReviewFlags(flags, &options)

	if err := flags.Parse(arguments); err != nil {
		return reviewOptions{}, err
	}

	if len(flags.Args()) != 0 {
		return reviewOptions{}, fmt.Errorf("usage: lore review [objective filters]")
	}

	if err := applyReviewFlags(&options); err != nil {
		return reviewOptions{}, err
	}

	return options, nil
}

func bindReviewFlags(flags *flag.FlagSet, options *reviewOptions) {
	flags.StringVar(&options.recordedBefore, "recorded-before", "", "entries recorded before an RFC3339 time")
	flags.StringVar(&options.verifiedBefore, "verified-before", "", "entries verified before an RFC3339 time")
	flags.Var(&options.statuses, "status", "entry lifecycle status")
	flags.Var(&options.contexts, "context", "required type:name or type:name@repository")
	flags.BoolVar(&options.params.DuplicateTitles, "duplicate-titles", false, "group duplicate canonical titles")
	flags.IntVar(&options.params.Limit, "limit", 0, "maximum returned candidates")
	flags.BoolVar(&options.jsonOutput, "json", false, "write JSON output")
}

func applyReviewFlags(options *reviewOptions) error {
	var err error

	options.params.RecordedBefore, err = parseEntryTime("recorded-before", options.recordedBefore)
	if err != nil {
		return err
	}

	options.params.VerifiedBefore, err = parseEntryTime("verified-before", options.verifiedBefore)
	if err != nil {
		return err
	}

	options.params.Statuses = make([]EntryStatus, len(options.statuses))
	for index, status := range options.statuses {
		options.params.Statuses[index] = EntryStatus(status)
	}

	options.params.Contexts, err = parseEntryContexts(options.contexts)
	if err != nil {
		return err
	}

	_, err = canonicalReviewParams(options.params)

	return err
}

func writeReviewCandidate(writer io.Writer, candidate ReviewCandidate) error {
	_, err := fmt.Fprintf(
		writer,
		"%s %s %s %q [%s]\n",
		candidate.Entry.ID,
		candidate.Entry.Kind,
		candidate.Entry.Status,
		candidate.Entry.Title,
		strings.Join(candidate.Reasons, ","),
	)

	return err
}

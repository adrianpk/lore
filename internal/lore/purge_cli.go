package lore

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
)

type purgeOptions struct {
	params         PurgeParams
	ids            entryStringListFlag
	statuses       entryStringListFlag
	recordedBefore string
	jsonOutput     bool
}

func (app *App) runPurge(ctx context.Context, arguments []string) error {
	options, err := parsePurgeOptions(arguments, app.stderr)
	if err != nil {
		return err
	}

	var result PurgeResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "purge", options.params, &result); err != nil {
		return err
	}

	if options.jsonOutput {
		return writeEntryJSON(app.stdout, result)
	}

	return writePurgeResult(app.stdout, result)
}

func parsePurgeOptions(arguments []string, errorOutput io.Writer) (purgeOptions, error) {
	options := purgeOptions{}
	flags := flag.NewFlagSet("purge", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	bindPurgeFlags(flags, &options)

	if err := flags.Parse(arguments); err != nil {
		return purgeOptions{}, err
	}

	if len(flags.Args()) != 0 {
		return purgeOptions{}, fmt.Errorf("usage: lore purge (--dry-run | --confirm <token>) <selector>")
	}

	if err := applyPurgeFlags(&options); err != nil {
		return purgeOptions{}, err
	}

	return options, nil
}

func bindPurgeFlags(flags *flag.FlagSet, options *purgeOptions) {
	flags.Var(&options.ids, "id", "entry ID selecting its complete chain")
	flags.StringVar(&options.recordedBefore, "recorded-before", "", "effective entries recorded before an RFC3339 time")
	flags.Var(&options.statuses, "status", "effective entry lifecycle status")
	flags.BoolVar(&options.params.All, "all", false, "select every entry chain")
	flags.BoolVar(&options.params.DryRun, "dry-run", false, "return exact selection and confirmation token")
	flags.StringVar(&options.params.Confirm, "confirm", "", "matching dry-run confirmation token")
	flags.BoolVar(&options.jsonOutput, "json", false, "write JSON output")
}

func applyPurgeFlags(options *purgeOptions) error {
	options.params.IDs = make([]EntryID, len(options.ids))
	for index, id := range options.ids {
		options.params.IDs[index] = EntryID(id)
	}

	options.params.Statuses = make([]EntryStatus, len(options.statuses))
	for index, status := range options.statuses {
		options.params.Statuses[index] = EntryStatus(status)
	}

	recordedBefore, err := parseEntryTime("recorded-before", options.recordedBefore)
	if err != nil {
		return err
	}

	options.params.RecordedBefore = recordedBefore
	_, err = canonicalizePurgeParams(options.params)

	return err
}

func writePurgeResult(writer io.Writer, result PurgeResult) error {
	state := "selected"
	if result.Purged {
		state = "purged"
	}

	if _, err := fmt.Fprintf(
		writer,
		"%s %d chains containing %d revisions\n",
		state,
		result.ChainCount,
		result.RevisionCount,
	); err != nil {
		return err
	}

	for _, chain := range result.Chains {
		if _, err := fmt.Fprintf(writer, "%s %s\n", chain.RootID, strings.Join(entryIDStrings(chain.EntryIDs), ",")); err != nil {
			return err
		}
	}

	if result.ConfirmationToken != "" {
		_, err := fmt.Fprintf(writer, "confirm %s\n", result.ConfirmationToken)

		return err
	}

	return nil
}

func entryIDStrings(ids []EntryID) []string {
	values := make([]string, len(ids))
	for index, id := range ids {
		values[index] = string(id)
	}

	return values
}

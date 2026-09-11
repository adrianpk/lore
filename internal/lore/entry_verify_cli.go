package lore

import (
	"context"
	"flag"
	"fmt"
)

func (app *App) runEntryVerify(ctx context.Context, arguments []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	flags.SetOutput(app.stderr)

	var (
		params     VerifyEntriesParams
		jsonOutput bool
	)

	flags.BoolVar(&params.Scheduled, "scheduled", false, "mark service-triggered verification")
	flags.BoolVar(&jsonOutput, "json", false, "write JSON output")

	if err := flags.Parse(arguments); err != nil {
		return err
	}

	if len(flags.Args()) != 0 {
		return fmt.Errorf("usage: lore verify [--scheduled] [--json]")
	}

	var result VerifyEntriesResult
	if err := NewClient(app.config.SocketPath).Call(ctx, "verify", params, &result); err != nil {
		return err
	}

	if jsonOutput {
		return writeEntryJSON(app.stdout, result)
	}

	_, err := fmt.Fprintf(
		app.stdout,
		"verified %d revisions across %d effective entries\n",
		result.RevisionCount,
		result.EffectiveCount,
	)

	return err
}

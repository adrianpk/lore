package lore

import (
	"context"
	"flag"
	"fmt"
	"io"
)

func (app *App) runMigrate(ctx context.Context, arguments []string) error {
	params, err := parseMigrationParams(arguments, app.stderr)
	if err != nil {
		return err
	}

	result, err := NewMigrator().Migrate(ctx, params)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(
		app.stdout,
		"migrated %d events across %d segments\n",
		result.Manifest.Destination.EventCount,
		result.Manifest.Destination.SegmentCount,
	)

	return err
}

func parseMigrationParams(arguments []string, errorOutput io.Writer) (MigrationParams, error) {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(errorOutput)

	var params MigrationParams
	flags.StringVar(&params.Source, "source", "", "legacy journal root")
	flags.StringVar(&params.Destination, "destination", "", "new canonical journal root")
	flags.StringVar(&params.Manifest, "manifest", "", "new cutover manifest path")

	if err := flags.Parse(arguments); err != nil {
		return MigrationParams{}, err
	}

	if len(flags.Args()) != 0 || params.Source == "" || params.Destination == "" || params.Manifest == "" {
		return MigrationParams{}, fmt.Errorf(
			"usage: lore migrate --source <legacy-journal-root> --destination <canonical-journal-root> --manifest <path>",
		)
	}

	return params, nil
}

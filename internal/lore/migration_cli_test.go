package lore

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseMigrationParams(t *testing.T) {
	t.Parallel()

	want := MigrationParams{Source: "legacy", Destination: "canonical", Manifest: "manifest.json"}

	got, err := parseMigrationParams([]string{
		"--source", want.Source,
		"--destination", want.Destination,
		"--manifest", want.Manifest,
	}, &bytes.Buffer{})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("parseMigrationParams() = %#v, %v", got, err)
	}
}

func TestParseMigrationParamsRequiresEveryPath(t *testing.T) {
	t.Parallel()

	if _, err := parseMigrationParams([]string{"--source", "legacy"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected incomplete migration arguments to fail")
	}
}

func TestAppMigratesWithoutServiceConfiguration(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatalf("create source: %v", err)
	}

	writeLegacyJournal(t, source, representativeLegacyHistory(t)[0])

	destination := filepath.Join(root, "destination")
	manifest := filepath.Join(root, "manifest.json")

	var output bytes.Buffer

	app := NewApp(Config{}, &output, &bytes.Buffer{})

	err := app.Run(context.Background(), []string{
		"migrate", "--source", source, "--destination", destination, "--manifest", manifest,
	})
	if err != nil {
		t.Fatalf("run migrate: %v", err)
	}

	if !strings.Contains(output.String(), "migrated 1 events across 1 segments") {
		t.Fatalf("migration output = %q", output.String())
	}

	if _, err := os.Stat(manifest); err != nil {
		t.Fatalf("stat migration manifest: %v", err)
	}
}

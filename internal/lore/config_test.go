package lore

import (
	"path/filepath"
	"testing"
)

func TestConfigFromHome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		home    string
		want    Config
		wantErr bool
	}{
		{
			name: "user paths",
			home: "/home/tester",
			want: Config{
				ConfigRoot:            filepath.FromSlash("/home/tester/.config/lore"),
				FilesRoot:             filepath.FromSlash("/home/tester/.lore"),
				JournalRoot:           filepath.FromSlash("/home/tester/.lore/events"),
				PendingPath:           filepath.FromSlash("/home/tester/.lore/pending.json"),
				VerificationStatePath: filepath.FromSlash("/home/tester/.lore/verification.json"),
			},
		},
		{
			name:    "relative home",
			home:    "home/tester",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ConfigFromHome(test.home)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}

				return
			}

			if err != nil {
				t.Fatalf("ConfigFromHome() error = %v", err)
			}

			if got != test.want {
				t.Fatalf("ConfigFromHome() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestDefaultConfigIncludesRuntimeSocket(t *testing.T) {
	home := t.TempDir()
	runtimeDirectory := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("HOME", home)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDirectory)

	config, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig() error = %v", err)
	}

	if config.FilesRoot != filepath.Join(home, ".lore") {
		t.Fatalf("FilesRoot = %q", config.FilesRoot)
	}

	wantRuntimeRoot := filepath.Join(runtimeDirectory, "lore")
	if config.RuntimeRoot != wantRuntimeRoot || config.SocketPath != filepath.Join(wantRuntimeRoot, "lore.sock") {
		t.Fatalf("runtime config = %#v", config)
	}
}

func TestDefaultConfigRequiresRuntimeDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", "")

	if _, err := DefaultConfig(); err == nil {
		t.Fatal("expected runtime directory error")
	}
}

func TestDefaultConfigForMigrationDoesNotRequireRuntimeDirectory(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")

	config, err := DefaultConfigForCommand("migrate")
	if err != nil || config != (Config{}) {
		t.Fatalf("DefaultConfigForCommand() = %#v, %v", config, err)
	}
}

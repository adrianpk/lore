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
				ConfigRoot:       filepath.FromSlash("/home/tester/.config/lore"),
				DataRoot:         filepath.FromSlash("/home/tester/.local/share/lore"),
				EntriesRoot:      filepath.FromSlash("/home/tester/.local/share/lore/entries"),
				EntryPendingRoot: filepath.FromSlash("/home/tester/.local/share/lore/pending"),
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
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", runtimeDirectory)

	config, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig() error = %v", err)
	}

	if config.DataRoot != filepath.Join(home, ".local", "share", "lore") {
		t.Fatalf("DataRoot = %q", config.DataRoot)
	}

	wantRuntimeRoot := filepath.Join(runtimeDirectory, "lore")
	if config.RuntimeRoot != wantRuntimeRoot || config.SocketPath != filepath.Join(wantRuntimeRoot, "lore.sock") {
		t.Fatalf("runtime config = %#v", config)
	}
}

func TestDefaultConfigUsesXDGDataHome(t *testing.T) {
	home := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "data")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), "runtime"))

	config, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig() error = %v", err)
	}

	wantRoot := filepath.Join(dataHome, "lore")
	if config.DataRoot != wantRoot || config.EntriesRoot != filepath.Join(wantRoot, "entries") {
		t.Fatalf("data config = %#v", config)
	}
}

func TestDefaultConfigRejectsRelativeXDGDataHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "relative/data")
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), "runtime"))

	if _, err := DefaultConfig(); err == nil {
		t.Fatal("expected XDG data directory error")
	}
}

func TestDefaultConfigRequiresRuntimeDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	if _, err := DefaultConfig(); err == nil {
		t.Fatal("expected runtime directory error")
	}
}

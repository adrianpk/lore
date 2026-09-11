// Package lore implements the local Lore journal and its domain contracts.
package lore

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config contains the user-local paths owned by Lore.
type Config struct {
	ConfigRoot       string
	DataRoot         string
	EntriesRoot      string
	EntryPendingRoot string
	RuntimeRoot      string
	SocketPath       string
}

// DefaultConfigForCommand resolves Lore paths for one command.
func DefaultConfigForCommand(_ string) (Config, error) {
	return DefaultConfig()
}

// DefaultConfig resolves Lore paths from the current process environment.
func DefaultConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("resolve user home: %w", err)
	}

	config, err := ConfigFromHome(home)
	if err != nil {
		return Config{}, err
	}

	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome != "" {
		if !filepath.IsAbs(dataHome) {
			return Config{}, fmt.Errorf("resolve Lore data paths: XDG_DATA_HOME must be absolute")
		}

		config.setDataRoot(filepath.Join(filepath.Clean(dataHome), "lore"))
	}

	runtimeDirectory := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDirectory == "" {
		return Config{}, fmt.Errorf("resolve Lore runtime paths: XDG_RUNTIME_DIR is required")
	}

	config.RuntimeRoot = filepath.Join(filepath.Clean(runtimeDirectory), "lore")
	config.SocketPath = filepath.Join(config.RuntimeRoot, "lore.sock")

	return config, nil
}

// ConfigFromHome resolves Lore paths for a user home directory.
func ConfigFromHome(home string) (Config, error) {
	if !filepath.IsAbs(home) {
		return Config{}, fmt.Errorf("resolve configuration: home path must be absolute")
	}

	cleanHome := filepath.Clean(home)
	dataRoot := filepath.Join(cleanHome, ".local", "share", "lore")

	config := Config{
		ConfigRoot: filepath.Join(cleanHome, ".config", "lore"),
	}
	config.setDataRoot(dataRoot)

	return config, nil
}

func (config *Config) setDataRoot(root string) {
	config.DataRoot = root
	config.EntriesRoot = filepath.Join(root, "entries")
	config.EntryPendingRoot = filepath.Join(root, "pending")
}

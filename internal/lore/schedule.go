package lore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const verificationStateSchema = "lore/verification/v1"

type verificationState struct {
	SchemaVersion string `json:"schema_version"`
	FullMonth     string `json:"full_month"`
}

func scheduledVerificationPlan(path string, now time.Time) (bool, string, error) {
	month := now.In(time.Local).Format("2006-01")

	state, valid, err := readVerificationState(path)
	if err != nil {
		return false, "", err
	}

	return !valid || state.FullMonth != month, month, nil
}

func readVerificationState(path string) (verificationState, bool, error) {
	encoded, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return verificationState{}, false, nil
	}

	if err != nil {
		return verificationState{}, false, fmt.Errorf("read verification state: %w", err)
	}

	state, valid := decodeVerificationState(encoded)

	return state, valid, nil
}

func decodeVerificationState(encoded []byte) (verificationState, bool) {
	var state verificationState

	err := json.Unmarshal(encoded, &state)
	valid := err == nil && state.SchemaVersion == verificationStateSchema && validVerificationMonth(state.FullMonth)

	return state, valid
}

func validVerificationMonth(value string) bool {
	parsed, err := time.Parse("2006-01", value)

	return err == nil && parsed.Format("2006-01") == value
}

func recordScheduledFullVerification(path, month string) error {
	if path == "" {
		return fmt.Errorf("record verification state: path is required")
	}

	if !validVerificationMonth(month) {
		return fmt.Errorf("record verification state: invalid month %q", month)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create verification state directory: %w", err)
	}

	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("set verification state directory permissions: %w", err)
	}

	state := verificationState{SchemaVersion: verificationStateSchema, FullMonth: month}

	encoded, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode verification state: %w", err)
	}

	encoded = append(encoded, '\n')

	temporary, err := os.CreateTemp(filepath.Dir(path), ".verification-*.tmp")
	if err != nil {
		return fmt.Errorf("create verification state: %w", err)
	}

	temporaryPath := temporary.Name()
	if err := writeVerificationStateFile(temporary, encoded); err != nil {
		return errors.Join(err, temporary.Close(), removeFile(temporaryPath))
	}

	if err := temporary.Close(); err != nil {
		return errors.Join(fmt.Errorf("close verification state: %w", err), removeFile(temporaryPath))
	}

	if err := os.Rename(temporaryPath, path); err != nil {
		return errors.Join(fmt.Errorf("publish verification state: %w", err), removeFile(temporaryPath))
	}

	return syncDirectory(filepath.Dir(path))
}

func writeVerificationStateFile(file *os.File, encoded []byte) error {
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("set verification state permissions: %w", err)
	}

	if _, err := file.Write(encoded); err != nil {
		return fmt.Errorf("write verification state: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync verification state: %w", err)
	}

	return nil
}

package lore

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"
)

const crockfordBase32 = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var base32Radix = big.NewInt(32)

// EventID is a canonical ULID string.
type EventID string

// NewEventID generates a ULID for the supplied UTC recording time.
func NewEventID(recordedAt time.Time, entropy io.Reader) (EventID, error) {
	milliseconds := recordedAt.UnixMilli()
	if milliseconds < 0 || milliseconds > (1<<48)-1 {
		return "", fmt.Errorf("generate event ID: timestamp is outside the ULID range")
	}

	if entropy == nil {
		entropy = rand.Reader
	}

	var (
		data      [16]byte
		timestamp [8]byte
	)
	binary.BigEndian.PutUint64(timestamp[:], uint64(milliseconds))
	copy(data[:6], timestamp[2:])

	if _, err := io.ReadFull(entropy, data[6:]); err != nil {
		return "", fmt.Errorf("generate event ID entropy: %w", err)
	}

	return EventID(encodeULID(data)), nil
}

// Validate checks the canonical ULID representation.
func (id EventID) Validate() error {
	value := string(id)
	if len(value) != 26 {
		return fmt.Errorf("ULID must contain 26 characters")
	}

	if value[0] > '7' {
		return fmt.Errorf("ULID exceeds 128 bits")
	}

	for _, character := range value {
		if !strings.ContainsRune(crockfordBase32, character) {
			return fmt.Errorf("ULID contains invalid character %q", character)
		}
	}

	return nil
}

func (id EventID) timestampMilliseconds() (int64, error) {
	if err := id.Validate(); err != nil {
		return 0, err
	}

	var milliseconds uint64
	for _, character := range string(id)[:10] {
		milliseconds = milliseconds*32 + uint64(strings.IndexRune(crockfordBase32, character))
	}

	return int64(milliseconds), nil
}

func encodeULID(data [16]byte) string {
	value := new(big.Int).SetBytes(data[:])
	quotient := new(big.Int).Set(value)
	remainder := new(big.Int)
	encoded := [26]byte{}

	for index := len(encoded) - 1; index >= 0; index-- {
		quotient.QuoRem(quotient, base32Radix, remainder)

		encoded[index] = crockfordBase32[remainder.Int64()]
	}

	return string(encoded[:])
}

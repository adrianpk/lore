package lore

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"
	"time"
)

const crockfordBase32 = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var base32Radix = big.NewInt(32)

// NewEntryID generates a ULID for the supplied UTC recording time.
func NewEntryID(recordedAt time.Time, entropy io.Reader) (EntryID, error) {
	milliseconds := recordedAt.UnixMilli()
	if milliseconds < 0 || milliseconds > (1<<48)-1 {
		return "", fmt.Errorf("generate entry ID: timestamp is outside the ULID range")
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
		return "", fmt.Errorf("generate entry ID entropy: %w", err)
	}

	return EntryID(encodeULID(data)), nil
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

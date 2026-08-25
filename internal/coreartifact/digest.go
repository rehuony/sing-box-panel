// SPDX-License-Identifier: GPL-3.0-or-later

package coreartifact

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrInvalidSHA256 = errors.New("invalid SHA-256 digest")

// SHA256 is a validated binary digest. The backing array is private to avoid
// accidental construction from a short or malformed string.
type SHA256 struct {
	sum [32]byte
}

func NewSHA256(sum [32]byte) SHA256 {
	return SHA256{sum: sum}
}

func ParseSHA256(value string) (SHA256, error) {
	if len(value) != hex.EncodedLen(32) {
		return SHA256{}, fmt.Errorf("%w: expected 64 hexadecimal characters", ErrInvalidSHA256)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil {
		return SHA256{}, fmt.Errorf("%w: %v", ErrInvalidSHA256, err)
	}
	var sum [32]byte
	copy(sum[:], decoded)
	return NewSHA256(sum), nil
}

func (digest SHA256) Bytes() [32]byte { return digest.sum }

func (digest SHA256) IsZero() bool {
	return digest == SHA256{}
}

func (digest SHA256) String() string {
	return hex.EncodeToString(digest.sum[:])
}

func (digest SHA256) MarshalText() ([]byte, error) {
	return []byte(digest.String()), nil
}

func (digest *SHA256) UnmarshalText(text []byte) error {
	parsed, err := ParseSHA256(string(text))
	if err != nil {
		return err
	}
	*digest = parsed
	return nil
}

func (digest SHA256) MarshalJSON() ([]byte, error) {
	return json.Marshal(digest.String())
}

func (digest *SHA256) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("%w: expected a string: %v", ErrInvalidSHA256, err)
	}
	return digest.UnmarshalText([]byte(value))
}

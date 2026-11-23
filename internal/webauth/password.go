// SPDX-License-Identifier: GPL-3.0-or-later

// Package webauth holds the protections every authenticated management HTTP
// API shares, the panel's and the node console's: password hashing, login
// admission and rate limits, same-origin and CSRF checks, and security
// headers.
package webauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/alexedwards/argon2id"
)

// GeneratePassword creates a random credential for initialization and recovery.
func GeneratePassword() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

var (
	ErrPassword   = errors.New("password must contain 12–128 Unicode characters")
	ErrBusy       = errors.New("password verification is busy; try again")
	ErrHash       = errors.New("password hash must be a supported Argon2id hash")
	passwordSlots = make(chan struct{}, 2)
)

func ValidatePassword(password string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 || utf8.RuneCountInString(password) > 128 {
		return ErrPassword
	}
	return nil
}

// ValidateHash checks cost bounds without performing expensive password work.
func ValidateHash(hash string) error {
	if len(hash) > 512 {
		return ErrHash
	}
	params, salt, key, err := argon2id.DecodeHash(hash)
	if err != nil || params.Memory < 19*1024 || params.Memory > 64*1024 || params.Iterations < 2 || params.Iterations > 4 || params.Parallelism < 1 || params.Parallelism > 4 || len(salt) < 16 || len(salt) > 32 || len(key) != 32 {
		return ErrHash
	}
	return nil
}

func passwordWork(ctx context.Context, work func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case passwordSlots <- struct{}{}:
		defer func() { <-passwordSlots }()
	default:
		return ErrBusy
	}
	if err := work(); err != nil {
		return err
	}
	return ctx.Err()
}

func HashPassword(ctx context.Context, password string) (hash string, err error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	err = passwordWork(ctx, func() error {
		var hashErr error
		hash, hashErr = argon2id.CreateHash(password, &argon2id.Params{
			Memory: 19 * 1024, Iterations: 2, Parallelism: 1, SaltLength: 16, KeyLength: 32,
		})
		return hashErr
	})
	return hash, err
}

func VerifyPassword(ctx context.Context, password, hash string) (matches bool, err error) {
	if err := ValidateHash(hash); err != nil {
		return false, err
	}
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) > 128 {
		return false, nil
	}
	err = passwordWork(ctx, func() error {
		var compareErr error
		matches, compareErr = argon2id.ComparePasswordAndHash(password, hash)
		return compareErr
	})
	return matches, err
}

// ReadPassword reads a password from standard input. It removes only the
// final line ending, never password spaces.
func ReadPassword(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, 515))
	if err != nil {
		clear(data)
		return nil, errors.New("could not read password")
	}
	if len(data) > 514 {
		clear(data)
		return nil, ErrPassword
	}
	return bytes.TrimSuffix(bytes.TrimSuffix(data, []byte("\n")), []byte("\r")), nil
}

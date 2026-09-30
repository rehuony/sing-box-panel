// SPDX-License-Identifier: GPL-3.0-or-later

// Package auth owns administrator credential validation and password hashing.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/alexedwards/argon2id"
)

// GeneratePassword creates the random credential used for initialization and recovery.
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
	passwordSlots = make(chan struct{}, 2)
)

// NormalizeEmail treats the single administrator's email as a case-insensitive identifier.
func NormalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value || address.Name != "" || len(value) > 254 || !strings.Contains(value, "@") {
		return "", errors.New("email must be a valid email address")
	}
	return value, nil
}

func ValidatePassword(password string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 || utf8.RuneCountInString(password) > 128 {
		return ErrPassword
	}
	return nil
}

// ValidateHash checks cost bounds without performing expensive password work.
func ValidateHash(hash string) error {
	if len(hash) > 512 {
		return errors.New("invalid password hash")
	}
	params, salt, key, err := argon2id.DecodeHash(hash)
	if err != nil || params.Memory < 19*1024 || params.Memory > 64*1024 || params.Iterations < 2 || params.Iterations > 4 || params.Parallelism < 1 || params.Parallelism > 4 || len(salt) < 16 || len(salt) > 32 || len(key) != 32 {
		return errors.New("password_hash must be a supported Argon2id hash; use config hash-password")
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

func Fingerprint(email, hash string) [32]byte {
	return sha256.Sum256([]byte(email + "\x00" + hash))
}

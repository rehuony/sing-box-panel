// SPDX-License-Identifier: GPL-3.0-or-later

// Package auth owns the administrator's credentials and login sessions.
package auth

import (
	"crypto/sha256"
	"errors"
	"net/mail"
	"strings"
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

// Fingerprint identifies a credential, so changing it ends its sessions.
func Fingerprint(email, hash string) [32]byte {
	return sha256.Sum256([]byte(email + "\x00" + hash))
}

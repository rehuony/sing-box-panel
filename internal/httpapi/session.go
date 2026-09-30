// SPDX-License-Identifier: GPL-3.0-or-later
package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
)

func constantTimeTokenEqual(left, right string) bool {
	leftHash, rightHash := sha256.Sum256([]byte(left)), sha256.Sum256([]byte(right))
	return subtle.ConstantTimeCompare(leftHash[:], rightHash[:]) == 1
}

// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"errors"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func IsRevisionConflict(err error) bool {
	return errors.Is(err, store.ErrRevisionConflict)
}

func IsRevisionNotFound(err error) bool {
	return errors.Is(err, store.ErrCanonicalRevisionNotFound)
}

func IsTaskNotFound(err error) bool {
	return errors.Is(err, store.ErrTaskNotFound)
}

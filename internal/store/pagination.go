package store

import (
	"errors"
	"math"
	"strings"
	"time"
)

func validatePageOffset(offset int, hasCursor bool) error {
	if offset < 0 || offset > math.MaxInt32 || (offset > 0 && hasCursor) {
		return errors.New("page offset must be between 0 and 2147483647 and cannot be combined with a cursor")
	}
	return nil
}

const (
	defaultPageLimit = 50
	maximumPageLimit = 200
)

// CreatedAtCursor is an exclusive keyset cursor for newest-first lists.
type CreatedAtCursor struct {
	CreatedAt time.Time
	ID        string
}

func normalizePageLimit(limit int) (int, error) {
	if limit < 0 {
		return 0, errors.New("page limit must not be negative")
	}
	if limit == 0 {
		return defaultPageLimit, nil
	}
	if limit > maximumPageLimit {
		return 0, errors.New("page limit exceeds maximum of 200")
	}
	return limit, nil
}

func validateCreatedAtCursor(cursor *CreatedAtCursor) error {
	if cursor == nil {
		return nil
	}
	if cursor.CreatedAt.IsZero() {
		return errors.New("page cursor created_at is zero")
	}
	if strings.TrimSpace(cursor.ID) == "" {
		return errors.New("page cursor id is empty")
	}
	return nil
}

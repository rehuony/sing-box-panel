package store

import (
	"errors"
	"strings"
	"time"
)

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

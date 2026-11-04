package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/configuration"
)

func TestConfigurationSnapshotsRemainReadableByID(t *testing.T) {
	ctx := testContext(t)
	store := openTestStore(t, ctx)
	now := time.Date(2026, time.August, 28, 9, 0, 0, 0, time.UTC)
	for i := 1; i <= 3; i++ {
		_, err := saveTestConfiguration(ctx, store, int64(i-1),
			NewCanonicalRevision{
				ID:            fmt.Sprintf("revision-%d", i),
				SchemaVersion: configuration.SchemaVersion,
				Document:      json.RawMessage(fmt.Sprintf(`{"experimental":{"value":%d}}`, i)),
				CommandID:     fmt.Sprintf("command-%d", i),
				CreatedAt:     now.Add(time.Duration(i) * time.Second),
			},
		)
		if err != nil {
			t.Fatalf("SaveConfigurationFile(%d) error = %v", i, err)
		}
	}

	byID, err := store.GetCanonicalRevision(ctx, "revision-2")
	if err != nil {
		t.Fatalf("GetCanonicalRevision() error = %v", err)
	}
	byID.Document[0] = 'x'
	unchanged, err := store.GetCanonicalRevision(ctx, "revision-2")
	if err != nil {
		t.Fatalf("GetCanonicalRevision() after mutation error = %v", err)
	}
	if string(unchanged.Document) != `{"experimental":{"value":2}}` {
		t.Fatalf("stored canonical document = %s, want defensive copy", unchanged.Document)
	}
	if _, err := store.GetCanonicalRevision(ctx, "missing"); !errors.Is(err, ErrCanonicalRevisionNotFound) {
		t.Fatalf("missing revision error = %v, want ErrCanonicalRevisionNotFound", err)
	}
}

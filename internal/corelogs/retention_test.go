package corelogs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDefaultRetentionKeepsSevenUTCDatesWithoutFileCountCap(t *testing.T) {
	logs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	logs.now = func() time.Time { return now }
	for day := 15; day <= 23; day++ {
		if err := os.WriteFile(filepath.Join(logs.dir, fmt.Sprintf("2026-09-%02d-000.log", day)), []byte("INFO saved\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for sequence := 1; sequence <= 40; sequence++ {
		if err := os.WriteFile(filepath.Join(logs.dir, fmt.Sprintf("2026-09-23-%03d.log", sequence)), []byte("INFO saved\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := logs.Prune(); err != nil {
		t.Fatal(err)
	}
	files, err := logs.List()
	if err != nil || len(files) != 47 {
		t.Fatalf("default retention: %d files, %v", len(files), err)
	}
	if files[len(files)-1].Name != "2026-09-17-000.log" {
		t.Fatal(files[len(files)-1])
	}
	policy := DefaultPolicy()
	policy.MaxFiles = 3
	if err := logs.SetPolicy(policy); err != nil {
		t.Fatal(err)
	}
	files, err = logs.List()
	if err != nil || len(files) != 3 {
		t.Fatalf("live count cap: %d %v", len(files), err)
	}
}

func TestLiveSizeChangeRotatesWithoutTruncatingAndBoundsLargeWrites(t *testing.T) {
	logs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	logs.now = func() time.Time { return now }
	name := "2026-09-23-000.log"
	fullLogFile(t, logs.dir, name)
	policy := DefaultPolicy()
	policy.MaxFileBytes = 1 << 20
	if err := logs.SetPolicy(policy); err != nil {
		t.Fatal(err)
	}
	output := logs.Writer()
	if _, err := output.Write([]byte(strings.Repeat("INFO output remains readable\n", 100000))); err != nil {
		t.Fatal(err)
	}
	files, err := logs.List()
	if err != nil || len(files) < 3 {
		t.Fatal(files, err)
	}
	for _, file := range files {
		if file.Name == name {
			if file.Size != DefaultPolicy().MaxFileBytes {
				t.Fatal("existing file truncated")
			}
		} else if file.Size > policy.MaxFileBytes {
			t.Fatalf("large write exceeded cap: %+v", file)
		}
	}
	// An idle writer cannot keep an expired date forever.
	now = now.AddDate(0, 0, 8)
	if err := logs.Prune(); err != nil {
		t.Fatal(err)
	}
	files, err = logs.List()
	if err != nil || len(files) != 0 {
		t.Fatal(files, err)
	}
}

func TestConcurrentPolicyChangesWritesAndHistoricalDeletion(t *testing.T) {
	logs, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	logs.now = func() time.Time { return now }
	old := "2026-09-22-000.log"
	if err := os.WriteFile(filepath.Join(logs.dir, old), []byte("INFO history\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		writer := logs.Writer()
		for range 100 {
			if _, err := writer.Write([]byte("INFO concurrent output\n")); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 100 {
			policy := DefaultPolicy()
			policy.MaxFiles = i%2 + 1
			if err := logs.SetPolicy(policy); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		_ = logs.Delete(old)
		for range 100 {
			if _, err := logs.List(); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	wg.Wait()
	if _, err := logs.Read("2026-09-23-000.log", 0); err != nil {
		t.Fatal(err)
	}
}

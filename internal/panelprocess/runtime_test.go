// SPDX-License-Identifier: GPL-3.0-or-later
package panelprocess

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRuntimeCallUsesReadyOwnerSocketAndPropagatesCancellation(t *testing.T) {
	dir := shortDirectory(t)
	control, err := Listen(Status{DataDir: dir}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { control.StopAccepting(); control.Finish(nil) }()
	entered := make(chan struct{})
	canceled := make(chan struct{})
	control.SetRuntimeHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			return
		}
		if input.Action == "wait" {
			close(entered)
			<-r.Context().Done()
			close(canceled)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"status":{"observation_state":"stopped"}}}`))
	}))
	if _, err := CallRuntime(t.Context(), dir, map[string]string{"action": "stop"}); err == nil {
		t.Fatal("accepted before ready")
	}
	control.Ready("127.0.0.1:3000")
	raw, err := CallRuntime(t.Context(), dir, map[string]string{"action": "stop"})
	if err != nil || !json.Valid(raw) {
		t.Fatalf("runtime response: %s %v", raw, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := CallRuntime(ctx, dir, map[string]string{"action": "wait"}); result <- err }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never entered")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error: %v", err)
	}
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not canceled")
	}
}

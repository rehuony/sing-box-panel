// SPDX-License-Identifier: GPL-3.0-or-later
package webauth

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoginClientNormalizesPeers(t *testing.T) {
	for input, want := range map[string]string{
		"192.0.2.1:123":                 "192.0.2.1",
		"[::ffff:192.0.2.1]:456":        "192.0.2.1",
		"[2001:db8:abcd:1::1]:123":      "2001:db8:abcd:1::/64",
		"[2001:db8:abcd:1:ffff::2]:456": "2001:db8:abcd:1::/64",
		"[fe80::1%en0]:123":             "fe80::/64",
		"invalid":                       "unknown",
	} {
		if got := LoginClient(input); got != want {
			t.Errorf("%q: got %q want %q", input, got, want)
		}
	}
}

func TestLoginLimiterAtomicAdmission(t *testing.T) {
	limiter := NewLoginLimiter(time.Now)
	var admitted atomic.Int32
	var group sync.WaitGroup
	for range 100 {
		group.Go(func() {
			if ok, _ := limiter.Begin("one-peer"); ok {
				admitted.Add(1)
				limiter.Finish()
			}
		})
	}
	group.Wait()
	if n := admitted.Load(); n == 0 || n > LoginAttemptLimit {
		t.Fatalf("admitted %d attempts", n)
	}
	for i := int(admitted.Load()); i < LoginAttemptLimit; i++ {
		if ok, _ := limiter.Begin("one-peer"); !ok {
			t.Fatal("concurrency rejections consumed the peer budget")
		}
		limiter.Finish()
	}
	if ok, _ := limiter.Begin("one-peer"); ok {
		t.Fatal("exceeded peer budget")
	}
	if ok, _ := limiter.Begin("other-peer"); !ok {
		t.Fatal("work slot leaked")
	}
	limiter.Finish()
}

func TestLoginLimiterGlobalBudgetAndExpiry(t *testing.T) {
	limiter := NewLoginLimiter(time.Now)
	now := time.Now()
	limiter.now = func() time.Time { return now }
	for i := range loginGlobalLimit {
		if ok, _ := limiter.Begin(fmt.Sprint(i)); !ok {
			t.Fatalf("rejected peer %d", i)
		}
		limiter.Finish()
	}
	if ok, retry := limiter.Begin("fresh-peer"); ok || retry != time.Minute {
		t.Fatalf("global limit: %v %v", ok, retry)
	}
	now = now.Add(time.Minute)
	if ok, _ := limiter.Begin("fresh-peer"); !ok {
		t.Fatal("expired global budget did not recover")
	}
	limiter.Finish()
}

func TestLoginLimiterDoesNotEvictActiveKeys(t *testing.T) {
	limiter := NewLoginLimiter(time.Now)
	now := time.Now()
	limiter.now = func() time.Time { return now }
	for i := range maxLoginClients {
		if ok, _ := limiter.Fingerprint(fmt.Sprint(i)); !ok {
			t.Fatalf("rejected key %d", i)
		}
	}
	if ok, _ := limiter.Fingerprint("new-key"); ok {
		t.Fatal("accepted a key past capacity")
	}
	if ok, _ := limiter.Fingerprint("0"); !ok {
		t.Fatal("lost existing key")
	}
	if len(limiter.entries) != maxLoginClients {
		t.Fatal("live entries were evicted")
	}
	now = now.Add(time.Minute)
	if ok, _ := limiter.Fingerprint("new-key"); !ok {
		t.Fatal("expired entries were not cleaned")
	}
}

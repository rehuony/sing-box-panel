// SPDX-License-Identifier: GPL-3.0-or-later
package webauth

import (
	"errors"
	"strings"
	"testing"

	"github.com/alexedwards/argon2id"
)

func TestPasswordHashPolicyAndWhitespace(t *testing.T) {
	password := "  密码 with spaces  "
	hash, err := HashPassword(t.Context(), password)
	if err != nil {
		t.Fatal(err)
	}
	params, salt, key, err := argon2id.DecodeHash(hash)
	if err != nil || params.Memory != 19456 || params.Iterations != 2 || params.Parallelism != 1 || len(salt) != 16 || len(key) != 32 {
		t.Fatal("unexpected hash parameters", err)
	}
	for _, input := range []string{password, strings.TrimSpace(password), "wrong"} {
		match, err := VerifyPassword(t.Context(), input, hash)
		if err != nil || match != (input == password) {
			t.Fatal("password was altered", err)
		}
	}
	for _, password := range []string{"short", strings.Repeat("密", 129), string([]byte{0xff})} {
		if err := ValidatePassword(password); err == nil {
			t.Fatal("invalid password accepted")
		}
	}
	for _, password := range []string{strings.Repeat("密", 12), strings.Repeat("密", 128)} {
		if err := ValidatePassword(password); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHashBoundsAndVerificationConcurrency(t *testing.T) {
	const password = "test-administrator-password"
	hash, err := HashPassword(t.Context(), password)
	if err != nil {
		t.Fatal(err)
	}
	for _, unsafe := range []string{"", "token", strings.Replace(hash, "m=19456", "m=4294967295", 1), strings.Replace(hash, "t=2", "t=0", 1), strings.Replace(hash, "p=1", "p=0", 1)} {
		if err := ValidateHash(unsafe); err == nil {
			t.Fatal("unsafe hash accepted")
		}
	}
	for range cap(passwordSlots) {
		passwordSlots <- struct{}{}
	}
	defer func() {
		for range cap(passwordSlots) {
			<-passwordSlots
		}
	}()
	if _, err := VerifyPassword(t.Context(), password, hash); !errors.Is(err, ErrBusy) {
		t.Fatal("hash concurrency unbounded", err)
	}
}

// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func TestParseVersionOutputIsStrict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		output  string
		want    string
		wantErr bool
	}{
		{name: "banner", output: "sing-box version 1.13.19\n", want: "1.13.19"},
		{name: "leading blanks", output: "\n  \nsing-box version 1.12.8\n", want: "1.12.8"},
		{name: "missing", output: "\n", wantErr: true},
		{name: "prefix", output: "wrapper sing-box version 1.13.19\n", wantErr: true},
		{name: "suffix", output: "sing-box version 1.13.19 debug\n", wantErr: true},
		{name: "invalid exact", output: "sing-box version latest\n", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			actual, err := parseVersionOutput([]byte(test.output))
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseVersionOutput(%q) succeeded: %s", test.output, actual)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseVersionOutput: %v", err)
			}
			if actual.String() != test.want {
				t.Fatalf("version = %s, want %s", actual, test.want)
			}
		})
	}
}

func TestVerifyBinaryFileRejectsSymlink(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	binaryPath := filepath.Join(directory, "sing-box")
	binary := []byte("executable")
	if err := os.WriteFile(binaryPath, binary, 0o700); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := verifyBinaryFile(context.Background(), binaryPath, 1024); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(directory, "sing-box-link")
	if err := os.Symlink(binaryPath, symlinkPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if err := verifyBinaryFile(context.Background(), symlinkPath, 1024); !errors.Is(err, ErrArtifactFile) {
		t.Fatalf("symlink verification error = %v, want ErrArtifactFile", err)
	}
}

func TestVerifyBinaryFileHonorsCancellationAndBounds(t *testing.T) {
	t.Parallel()

	binaryPath := filepath.Join(t.TempDir(), "sing-box")
	binary := []byte("executable")
	if err := os.WriteFile(binaryPath, binary, 0o700); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := verifyBinaryFile(ctx, binaryPath, 1024); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled verification error = %v, want context.Canceled", err)
	}
	if err := verifyBinaryFile(context.Background(), binaryPath, int64(len(binary)-1)); !errors.Is(err, ErrArtifactFile) {
		t.Fatalf("bounded verification error = %v, want ErrArtifactFile", err)
	}
}

func TestCloneAndValidateBundleCopiesConfigAndRejectsAmbiguity(t *testing.T) {
	t.Parallel()

	fixture := newRuntimeFixture(t, "1.13.19", []byte(`{"route":{}}`))
	prepared, err := cloneAndValidateBundle(fixture.bundle, 1024)
	if err != nil {
		t.Fatalf("cloneAndValidateBundle: %v", err)
	}
	fixture.bundle.StartupConfig[0] = '!'
	if prepared.StartupConfig[0] == '!' {
		t.Fatal("prepared config aliases caller-owned bytes")
	}

	invalid := prepared
	invalid.BinaryPath = filepath.Join("relative", "sing-box")
	if _, err := cloneAndValidateBundle(invalid, 1024); !errors.Is(err, ErrInvalidBundle) {
		t.Fatalf("relative path error = %v, want ErrInvalidBundle", err)
	}
	invalid = prepared
	invalid.ExactVersion = coreartifact.ExactVersion{}
	if _, err := cloneAndValidateBundle(invalid, 1024); !errors.Is(err, ErrInvalidBundle) {
		t.Fatalf("zero version error = %v, want ErrInvalidBundle", err)
	}
	invalid = prepared
	invalid.ID = "unsafe\nbundle"
	if _, err := cloneAndValidateBundle(invalid, 1024); !errors.Is(err, ErrInvalidBundle) {
		t.Fatalf("control-character ID error = %v, want ErrInvalidBundle", err)
	}
}

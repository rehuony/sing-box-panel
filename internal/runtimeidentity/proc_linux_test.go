// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package runtimeidentity

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rehuony/sing-box-panel/internal/store"
)

func TestProcInspectorBindsPIDIncarnationAndExecutableInode(t *testing.T) {
	root := t.TempDir()
	processDirectory := filepath.Join(root, "42")
	if err := os.Mkdir(processDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	fields := make([]string, 20)
	fields[0] = "S"
	for index := 1; index < 19; index++ {
		fields[index] = "0"
	}
	fields[19] = "987654"
	if err := os.WriteFile(filepath.Join(processDirectory, "stat"), []byte("42 (sing box worker) "+strings.Join(fields, " ")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(root, "sing-box")
	if err := os.WriteFile(binaryPath, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(binaryPath, filepath.Join(processDirectory, "exe")); err != nil {
		t.Fatal(err)
	}
	inspector := procInspector{root: root}
	observation := store.RuntimeObservation{
		PID: 42, ProcessStartToken: "987654", CoreArtifactID: "core-a",
		ExactCoreVersion: "1.13.19", ArchiveSHA256: strings.Repeat("a", 64),
		BinarySHA256: strings.Repeat("b", 64),
	}
	artifact := store.CoreArtifact{
		ID: "core-a", ExactVersion: "1.13.19", ReportedVersion: "1.13.19",
		ArchiveSHA256: strings.Repeat("a", 64), BinarySHA256: strings.Repeat("b", 64), BinaryPath: binaryPath,
	}
	if err := inspector.Verify(context.Background(), observation, artifact); err != nil {
		t.Fatal(err)
	}
	observation.ProcessStartToken = "old"
	if err := inspector.Verify(context.Background(), observation, artifact); err == nil {
		t.Fatal("Verify accepted a reused PID incarnation")
	}
}

//go:build linux

// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func TestLinuxExecutorOwnsAndTerminatesProcessGroup(t *testing.T) {
	t.Parallel()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	defer reader.Close()
	executor, err := newDefaultCommandExecutor()
	if err != nil {
		t.Fatalf("newDefaultCommandExecutor: %v", err)
	}
	process, err := executor.Start(Command{
		Path:   os.Args[0],
		Args:   []string{"-test.run=^TestLinuxProcessGroupHelper$"},
		Dir:    t.TempDir(),
		Env:    append(os.Environ(), "SING_BOX_PANEL_RUNTIME_HELPER=1"),
		Stdout: writer,
		Stderr: writer,
	})
	if err != nil {
		writer.Close()
		t.Fatalf("Start: %v", err)
	}
	processGroupID, err := syscall.Getpgid(process.PID())
	if err != nil {
		_ = process.Kill()
		writer.Close()
		t.Fatalf("Getpgid: %v", err)
	}
	if processGroupID != process.PID() {
		_ = process.Kill()
		writer.Close()
		t.Fatalf("process group ID = %d, want child PID %d", processGroupID, process.PID())
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close(writer): %v", err)
	}

	ready := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(reader).ReadString('\n')
		ready <- line
	}()
	select {
	case line := <-ready:
		if line != "ready\n" {
			_ = process.Kill()
			t.Fatalf("helper readiness = %q", line)
		}
	case <-time.After(5 * time.Second):
		_ = process.Kill()
		t.Fatal("timed out waiting for helper readiness")
	}
	if err := process.Terminate(); err != nil {
		_ = process.Kill()
		t.Fatalf("Terminate: %v", err)
	}
	waited := make(chan error, 1)
	go func() { waited <- process.Wait() }()
	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("Wait after group SIGTERM: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = process.Kill()
		t.Fatal("timed out waiting for helper exit")
	}
}

func TestLinuxProcessGroupHelper(t *testing.T) {
	if os.Getenv("SING_BOX_PANEL_RUNTIME_HELPER") != "1" {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	fmt.Println("ready")
	<-signals
	os.Exit(0)
}

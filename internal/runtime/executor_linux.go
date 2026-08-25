//go:build linux

// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

type platformExecutor struct{}

func newDefaultCommandExecutor() (CommandExecutor, error) {
	return platformExecutor{}, nil
}

func (platformExecutor) Run(
	ctx context.Context,
	command Command,
	maximumOutput int64,
) ([]byte, error) {
	if ctx == nil || command.Path == "" || maximumOutput <= 0 {
		return nil, errors.New("invalid runtime command")
	}
	cmd := exec.CommandContext(ctx, command.Path, command.Args...)
	configureCommand(cmd, command)
	output := &boundedBuffer{maximum: maximumOutput}
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if output.Overflowed() {
		return nil, ErrCommandOutputTooLarge
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func (platformExecutor) Start(command Command) (ChildProcess, error) {
	if command.Path == "" {
		return nil, errors.New("runtime command path is empty")
	}
	cmd := exec.Command(command.Path, command.Args...)
	configureCommand(cmd, command)
	if cmd.Stdout == nil {
		cmd.Stdout = io.Discard
	}
	if cmd.Stderr == nil {
		cmd.Stderr = io.Discard
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &linuxChildProcess{command: cmd, pid: cmd.Process.Pid}, nil
}

func configureCommand(cmd *exec.Cmd, command Command) {
	cmd.Dir = command.Dir
	cmd.Env = append([]string(nil), command.Env...)
	cmd.Stdout = command.Stdout
	cmd.Stderr = command.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}
}

type linuxChildProcess struct {
	command *exec.Cmd
	pid     int
}

func (process *linuxChildProcess) PID() int { return process.pid }

func (process *linuxChildProcess) Wait() error { return process.command.Wait() }

func (process *linuxChildProcess) Terminate() error {
	return signalProcessGroup(process.pid, syscall.SIGTERM)
}

func (process *linuxChildProcess) Kill() error {
	return signalProcessGroup(process.pid, syscall.SIGKILL)
}

func signalProcessGroup(pid int, signal syscall.Signal) error {
	if pid <= 0 {
		return errors.New("invalid process id")
	}
	if err := syscall.Kill(-pid, signal); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return ErrProcessExited
		}
		return fmt.Errorf("signal process group: %w", err)
	}
	return nil
}

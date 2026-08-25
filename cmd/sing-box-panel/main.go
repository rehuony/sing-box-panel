// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/rehuony/sing-box-panel/internal/application"
	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/cli"
	"github.com/rehuony/sing-box-panel/internal/server"
	webui "github.com/rehuony/sing-box-panel/web"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	var signalExit atomic.Int32
	watcherDone := make(chan struct{})
	defer close(watcherDone)
	go watchSignalContext(signals, watcherDone, cancel, &signalExit)

	build := buildinfo.Info{Version: version, Commit: commit, Date: date}
	root := cli.NewRootCommand(cli.Dependencies{
		Stdin:           os.Stdin,
		Stdout:          os.Stdout,
		Stderr:          os.Stderr,
		Build:           build,
		OpenApplication: application.Open,
		RunServer: func(ctx context.Context, path string) error {
			return server.Run(ctx, path, build, webui.Assets())
		},
	})
	if err := root.ExecuteContext(ctx); err != nil {
		_ = cli.WriteError(os.Stderr, root, err)
		if code := signalExit.Load(); code != 0 {
			return int(code)
		}
		return cli.ExitCode(err)
	}
	if code := signalExit.Load(); code != 0 {
		return int(code)
	}
	return 0
}

func watchSignalContext(
	signals <-chan os.Signal,
	done <-chan struct{},
	cancel context.CancelFunc,
	exitCode *atomic.Int32,
) {
	select {
	case received := <-signals:
		if received == syscall.SIGTERM {
			exitCode.Store(143)
		} else {
			exitCode.Store(130)
		}
		cancel()
	case <-done:
	}
}

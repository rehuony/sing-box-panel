// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rehuony/sing-box-panel/internal/buildinfo"
	"github.com/rehuony/sing-box-panel/internal/cliui"
	"github.com/rehuony/sing-box-panel/internal/sing-box-node/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	root := cli.NewRootCommand(cli.Dependencies{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Build: buildinfo.Current()})
	err := root.ExecuteContext(ctx)
	stop()
	if err != nil {
		_ = cliui.WriteError(os.Stderr, root, err)
		os.Exit(cliui.ExitCode(err))
	}
}

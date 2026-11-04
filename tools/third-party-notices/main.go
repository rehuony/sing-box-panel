// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

type commandDependencies struct {
	findRoot func() (string, error)
	generate func(context.Context, string) ([]byte, error)
	readFile func(string) ([]byte, error)
	write    func(string, []byte) error
}

func run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	return runWithDependencies(ctx, arguments, stdout, stderr, commandDependencies{
		findRoot: findRepositoryRoot,
		generate: generateNotices,
		readFile: os.ReadFile,
		write:    writeAtomically,
	})
}

func runWithDependencies(
	ctx context.Context,
	arguments []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies commandDependencies,
) int {
	flags := flag.NewFlagSet("third-party-notices", flag.ContinueOnError)
	flags.SetOutput(stderr)
	check := flags.Bool("check", false, "fail if THIRD_PARTY_NOTICES is not current")
	output := flags.String("output", "THIRD_PARTY_NOTICES", "notice file path, relative to the repository root")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected positional arguments: %v\n", flags.Args())
		return 2
	}

	root, err := dependencies.findRoot()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	generated, err := dependencies.generate(ctx, root)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "generate third-party notices: %v\n", err)
		return 1
	}
	outputPath := *output
	if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(root, outputPath)
	}
	if *check {
		current, err := dependencies.readFile(outputPath)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "read committed notices: %v\n", err)
			return 1
		}
		if !bytes.Equal(current, generated) {
			_, _ = fmt.Fprintf(stderr, "%s is stale; run make notices\n", outputPath)
			return 1
		}
		_, _ = fmt.Fprintf(stdout, "third-party notices are current: %s\n", outputPath)
		return 0
	}
	if err := dependencies.write(outputPath, generated); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "generated %s\n", outputPath)
	return 0
}

func findRepositoryRoot() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	for {
		if regularFile(filepath.Join(directory, "go.mod")) && regularFile(filepath.Join(directory, "web", "pnpm-lock.yaml")) {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("cannot find repository root containing go.mod and web/pnpm-lock.yaml")
		}
		directory = parent
	}
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func writeAtomically(path string, content []byte) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".third-party-notices.*")
	if err != nil {
		return fmt.Errorf("create temporary notices file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return fmt.Errorf("set temporary notices permissions: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary notices: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary notices: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary notices: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace notices file: %w", err)
	}
	return nil
}

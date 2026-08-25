// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rehuony/sing-box-panel/internal/notices"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string) error {
	flags := flag.NewFlagSet("third-party-notices", flag.ContinueOnError)
	check := flags.Bool("check", false, "fail if THIRD_PARTY_NOTICES is not current")
	output := flags.String("output", "THIRD_PARTY_NOTICES", "notice file path, relative to the repository root")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}

	root, err := findRepositoryRoot()
	if err != nil {
		return err
	}
	generated, err := notices.Generate(ctx, root)
	if err != nil {
		return fmt.Errorf("generate third-party notices: %w", err)
	}
	outputPath := *output
	if !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(root, outputPath)
	}
	if *check {
		current, err := os.ReadFile(outputPath)
		if err != nil {
			return fmt.Errorf("read committed notices: %w", err)
		}
		if !bytes.Equal(current, generated) {
			return fmt.Errorf("%s is stale; run go run ./cmd/third-party-notices", outputPath)
		}
		fmt.Printf("third-party notices are current: %s\n", outputPath)
		return nil
	}
	if err := writeAtomically(outputPath, generated); err != nil {
		return err
	}
	fmt.Printf("generated %s\n", outputPath)
	return nil
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

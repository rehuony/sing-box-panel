// SPDX-License-Identifier: GPL-3.0-or-later

// Command verify-openapi validates the repository's offline OpenAPI contract.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/rehuony/sing-box-panel/internal/openapiverify"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) != 1 {
		fmt.Fprintln(stderr, "usage: verify-openapi OPENAPI_YAML")
		return 2
	}

	path := arguments[0]
	findings := openapiverify.ValidateFile(path)
	if len(findings) != 0 {
		for _, finding := range findings {
			fmt.Fprintf(stderr, "OpenAPI validation error: %v\n", finding)
		}
		return 1
	}
	fmt.Fprintf(stdout, "OpenAPI validation passed: %s\n", path)
	return 0
}

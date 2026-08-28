// SPDX-License-Identifier: GPL-3.0-or-later

package singbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/rehuony/sing-box-panel/internal/artifactstore"
	"github.com/rehuony/sing-box-panel/internal/configuration"
)

const (
	contractBinaryEnvironment       = "SING_BOX_CONTRACT_BINARY"
	contractVersionEnvironment      = "SING_BOX_CONTRACT_VERSION"
	contractArchitectureEnvironment = "SING_BOX_CONTRACT_ARCHITECTURE"
	contractRequiredEnvironment     = "SING_BOX_CONTRACT_REQUIRED"
	maximumContractOutput           = 64 << 10
)

func TestCompiledAdaptersAcceptExactOfficialBinary(t *testing.T) {
	binaryPath, expectedVersion, expectedArchitecture := exactCoreContractInput(t)
	if runtime.GOOS != "linux" || runtime.GOARCH != expectedArchitecture {
		t.Fatalf("contract runner = %s/%s, want linux/%s", runtime.GOOS, runtime.GOARCH, expectedArchitecture)
	}

	absoluteBinary, err := filepath.Abs(binaryPath)
	if err != nil {
		t.Fatalf("resolve contract binary: %v", err)
	}
	info, err := os.Stat(absoluteBinary)
	if err != nil {
		t.Fatalf("inspect contract binary: %v", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		t.Fatal("contract binary must be a regular executable file")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := (artifactstore.ExecVersionInspector{}).Inspect(ctx, absoluteBinary, maximumContractOutput)
	if err != nil {
		t.Fatalf("inspect exact sing-box binary: %v", err)
	}
	if got := report.Version.String(); got != expectedVersion {
		t.Fatalf("reported version = %q, want %q", got, expectedVersion)
	}
	fingerprint, err := report.FeatureFingerprint.CanonicalJSON()
	if err != nil {
		t.Fatalf("encode reported feature fingerprint: %v", err)
	}

	registry := NewConfigurationRegistry()
	profile := configuration.CoreProfile{
		ExactVersion:       expectedVersion,
		OperatingSystem:    "linux",
		Architecture:       expectedArchitecture,
		Variant:            "plain",
		FeatureFingerprint: fingerprint,
	}
	resolved, err := registry.Resolve(profile)
	if err != nil {
		t.Fatalf("resolve exact compiled adapter: %v", err)
	}
	if resolved.ExactVersion() != expectedVersion {
		t.Fatalf("resolved version = %q, want %q", resolved.ExactVersion(), expectedVersion)
	}
	projection, err := registry.Project(profile, configuration.ProjectionRequest{
		CanonicalJSON: []byte(`{"schema_version":2,"configuration":{"log":{"disabled":true},"inbounds":[{"_panel":{"id":"contract-mixed","enabled":true},"type":"mixed","tag":"contract-mixed","listen":"127.0.0.1","listen_port":19090}]}}`),
	})
	if err != nil {
		t.Fatalf("project compatible canonical configuration: %v", err)
	}

	configurationDirectory := t.TempDir()
	configurationPath := filepath.Join(configurationDirectory, "config.json")
	if err := os.WriteFile(configurationPath, projection.ConfigJSON, 0o600); err != nil {
		t.Fatalf("write projected configuration: %v", err)
	}
	command := exec.CommandContext(ctx, absoluteBinary, "check", "-c", configurationPath)
	command.Env = []string{"HOME=" + configurationDirectory, "LANG=C", "LC_ALL=C", "PATH=/usr/bin:/bin"}
	command.Dir = configurationDirectory
	output := &contractOutput{maximum: maximumContractOutput}
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		t.Fatalf("sing-box check failed: %v\n%s", err, output.Bytes())
	}
}

func exactCoreContractInput(t *testing.T) (string, string, string) {
	t.Helper()
	requiredValue := os.Getenv(contractRequiredEnvironment)
	if requiredValue != "" && requiredValue != "1" {
		t.Fatalf("%s must be empty or 1", contractRequiredEnvironment)
	}
	required := requiredValue == "1"
	binaryPath := os.Getenv(contractBinaryEnvironment)
	expectedVersion := os.Getenv(contractVersionEnvironment)
	expectedArchitecture := os.Getenv(contractArchitectureEnvironment)
	if binaryPath == "" && expectedVersion == "" && expectedArchitecture == "" && !required {
		t.Skip("exact sing-box contract binary was not provided")
	}
	for name, value := range map[string]string{
		contractBinaryEnvironment:       binaryPath,
		contractVersionEnvironment:      expectedVersion,
		contractArchitectureEnvironment: expectedArchitecture,
	} {
		if value == "" {
			t.Fatalf("%s is required for the exact core contract", name)
		}
	}
	if expectedArchitecture != "amd64" && expectedArchitecture != "arm64" {
		t.Fatalf("%s = %q, want amd64 or arm64", contractArchitectureEnvironment, expectedArchitecture)
	}
	return binaryPath, expectedVersion, expectedArchitecture
}

type contractOutput struct {
	buffer  bytes.Buffer
	written int
	maximum int
}

func (output *contractOutput) Write(data []byte) (int, error) {
	if len(data) > output.maximum-output.written {
		return 0, fmt.Errorf("contract output exceeds %d bytes", output.maximum)
	}
	written, err := output.buffer.Write(data)
	output.written += written
	return written, err
}

func (output *contractOutput) Bytes() []byte { return output.buffer.Bytes() }

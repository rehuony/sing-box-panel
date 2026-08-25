// SPDX-License-Identifier: GPL-3.0-or-later

package artifactstore

import (
	"bytes"
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rehuony/sing-box-panel/internal/coreartifact"
)

func verifyELF(binaryPath string, expected coreartifact.Architecture) error {
	file, err := elf.Open(binaryPath)
	if err != nil {
		return fail(StepELF, "parse", errors.Join(ErrELF, err))
	}
	defer file.Close()
	if file.Class != elf.ELFCLASS64 || file.Data != elf.ELFDATA2LSB ||
		(file.Type != elf.ET_EXEC && file.Type != elf.ET_DYN) ||
		(file.OSABI != elf.ELFOSABI_NONE && file.OSABI != elf.ELFOSABI_LINUX) {
		return fail(StepELF, "unsupported_format", ErrELF)
	}
	wantMachine := elf.EM_NONE
	switch expected {
	case coreartifact.ArchitectureAMD64:
		wantMachine = elf.EM_X86_64
	case coreartifact.ArchitectureARM64:
		wantMachine = elf.EM_AARCH64
	default:
		return fail(StepELF, "unsupported_architecture", ErrELF)
	}
	if file.Machine != wantMachine {
		return fail(StepELF, "architecture_mismatch", ErrELF)
	}
	return nil
}

type ExecVersionInspector struct {
	Timeout time.Duration
}

func (inspector ExecVersionInspector) Inspect(
	ctx context.Context,
	binaryPath string,
	maximumOutput int64,
) (VersionReport, error) {
	if ctx == nil || binaryPath == "" || maximumOutput <= 0 {
		return VersionReport{}, fail(StepVersion, "invalid_request", ErrVersion)
	}
	timeout := inspector.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	operationContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(operationContext, binaryPath, "version")
	command.Env = []string{"LANG=C", "LC_ALL=C", "PATH=/usr/bin:/bin"}
	command.Dir = filepath.Dir(binaryPath)
	command.WaitDelay = time.Second
	if err := configureVersionCommand(command); err != nil {
		return VersionReport{}, fail(StepVersion, "execution_setup", errors.Join(ErrVersion, err))
	}
	output := &boundedBuffer{maximum: maximumOutput}
	command.Stdout = output
	command.Stderr = output
	runErr := command.Run()
	terminateErr := terminateVersionCommand(command)
	if runErr != nil || terminateErr != nil {
		if operationContext.Err() != nil {
			return VersionReport{}, fail(StepVersion, "cancelled", operationContext.Err())
		}
		return VersionReport{}, fail(StepVersion, "execution", errors.Join(ErrVersion, runErr, terminateErr))
	}
	report, err := parseVersionOutput(output.Bytes())
	if err != nil {
		return VersionReport{}, fail(StepVersion, "output", errors.Join(ErrVersion, err))
	}
	return report, nil
}

type boundedBuffer struct {
	buffer  bytes.Buffer
	written int64
	maximum int64
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	remaining := buffer.maximum - buffer.written
	if remaining <= 0 || int64(len(data)) > remaining {
		return 0, ErrTooLarge
	}
	written, err := buffer.buffer.Write(data)
	buffer.written += int64(written)
	return written, err
}

func (buffer *boundedBuffer) Bytes() []byte { return buffer.buffer.Bytes() }

const (
	maximumVersionBannerBytes = 64 << 10
	maximumVersionBannerLines = 64
	maximumVersionLineBytes   = 4 << 10
)

func parseVersionOutput(output []byte) (VersionReport, error) {
	if len(output) > maximumVersionBannerBytes {
		return VersionReport{}, fmt.Errorf("version banner: %w", ErrTooLarge)
	}
	if !utf8.Valid(output) {
		return VersionReport{}, errors.New("version banner is not UTF-8")
	}
	lines := strings.Split(string(output), "\n")
	if len(lines) > maximumVersionBannerLines+1 {
		return VersionReport{}, errors.New("version banner has too many lines")
	}

	report := VersionReport{FeatureFingerprint: UnknownFeatureFingerprint()}
	foundVersion := false
	seenMetadata := make(map[string]struct{})
	for _, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) > maximumVersionLineBytes || hasUnsafeVersionCharacter(line) {
			return VersionReport{}, errors.New("version banner contains an invalid line")
		}
		if !foundVersion {
			fields := strings.Fields(line)
			if len(fields) != 3 || fields[0] != "sing-box" || fields[1] != "version" {
				return VersionReport{}, errors.New("unexpected version banner")
			}
			version, err := coreartifact.ParseExactVersion(fields[2])
			if err != nil {
				return VersionReport{}, err
			}
			report.Version = version
			foundVersion = true
			continue
		}

		separator := strings.IndexByte(line, ':')
		if separator <= 0 {
			return VersionReport{}, errors.New("malformed version metadata")
		}
		label := line[:separator]
		value := strings.TrimSpace(line[separator+1:])
		if !validVersionMetadataLabel(label) || value == "" {
			return VersionReport{}, errors.New("malformed version metadata")
		}
		if _, duplicate := seenMetadata[label]; duplicate {
			return VersionReport{}, errors.New("duplicate version metadata")
		}
		seenMetadata[label] = struct{}{}

		switch label {
		case "Tags":
			features, err := parseReportedFeatures(value)
			if err != nil {
				return VersionReport{}, err
			}
			report.FeatureFingerprint, err = newReportedFeatureFingerprint(features)
			if err != nil {
				return VersionReport{}, err
			}
		case "CGO":
			if value != "enabled" && value != "disabled" {
				return VersionReport{}, errors.New("invalid CGO version metadata")
			}
		}
	}
	if !foundVersion {
		return VersionReport{}, errors.New("missing version banner")
	}
	return report, nil
}

func parseReportedFeatures(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	features := make([]string, 0, len(parts))
	for _, part := range parts {
		feature := strings.TrimSpace(part)
		if feature == "" {
			return nil, errors.New("empty reported feature")
		}
		features = append(features, feature)
	}
	return features, nil
}

func hasUnsafeVersionCharacter(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func validVersionMetadataLabel(value string) bool {
	if value == "" || len(value) > 64 || value[0] < 'A' || value[0] > 'Z' {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if (character >= 'A' && character <= 'Z') ||
			(character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') || character == '-' {
			continue
		}
		return false
	}
	return true
}

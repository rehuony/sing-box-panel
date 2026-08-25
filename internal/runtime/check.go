// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
)

// Check verifies the exact binary identity and immutable config bytes using
// sing-box's own version and check commands. It never starts or replaces the
// managed child process.
func (manager *Manager) Check(ctx context.Context, bundle AppliedBundle) error {
	if ctx == nil {
		return fail("check", "invalid_context", ErrInvalidBundle, nil)
	}
	prepared, err := cloneAndValidateBundle(bundle, manager.options.MaximumConfigBytes)
	if err != nil {
		return fail("check", "invalid_bundle", ErrInvalidBundle, err)
	}
	manager.operationMu.Lock()
	defer manager.operationMu.Unlock()
	manager.mu.Lock()
	blocked := manager.closing || manager.closed
	manager.mu.Unlock()
	if blocked {
		return ErrClosed
	}
	return manager.checkLocked(ctx, prepared)
}

func (manager *Manager) checkLocked(ctx context.Context, bundle AppliedBundle) error {
	if err := contextError(ctx); err != nil {
		return fail("check", "cancelled", err, err)
	}
	if err := verifyStartupConfigDigest(bundle.StartupConfig, bundle.StartupConfigDigest); err != nil {
		return fail("verify_config", "digest_mismatch", ErrStartupConfigDigest, err)
	}
	if _, err := verifyBinaryDigest(
		ctx, bundle.BinaryPath, bundle.ArtifactDigest, manager.options.MaximumBinaryBytes,
	); err != nil {
		if ctx.Err() != nil {
			return fail("check", "cancelled", ctx.Err(), err)
		}
		return fail("verify_artifact", "digest_mismatch", ErrArtifactDigest, err)
	}
	versionOutput, err := manager.options.Executor.Run(
		ctx,
		manager.command(bundle.BinaryPath, "version"),
		manager.options.MaximumCommandOutput,
	)
	if err != nil {
		if ctx.Err() != nil {
			return fail("check", "cancelled", ctx.Err(), err)
		}
		return fail("verify_version", "execution", ErrVersionMismatch, err)
	}
	actualVersion, err := parseVersionOutput(versionOutput)
	if err != nil {
		return fail("verify_version", "invalid_output", ErrVersionMismatch, err)
	}
	if actualVersion != bundle.ExactVersion {
		return fail("verify_version", "mismatch", ErrVersionMismatch, nil)
	}
	if _, err := verifyBinaryDigest(
		ctx, bundle.BinaryPath, bundle.ArtifactDigest, manager.options.MaximumBinaryBytes,
	); err != nil {
		return fail("verify_artifact", "changed_after_version", ErrArtifactDigest, err)
	}
	configPath, err := materializeStartupConfig(
		manager.options.RuntimeDir, bundle.StartupConfigDigest, bundle.StartupConfig,
	)
	if err != nil {
		return fail("materialize_config", "write", ErrMaterialization, err)
	}
	if _, err := manager.options.Executor.Run(
		ctx,
		manager.command(bundle.BinaryPath, "check", "-c", configPath),
		manager.options.MaximumCommandOutput,
	); err != nil {
		if ctx.Err() != nil {
			return fail("check", "cancelled", ctx.Err(), err)
		}
		return fail("check_config", "rejected", ErrCheckFailed, err)
	}
	if _, err := verifyBinaryDigest(
		ctx, bundle.BinaryPath, bundle.ArtifactDigest, manager.options.MaximumBinaryBytes,
	); err != nil {
		return fail("verify_artifact", "changed_after_check", ErrArtifactDigest, err)
	}
	if err := contextError(ctx); err != nil {
		return fail("check", "cancelled", err, errors.Join(ErrCheckFailed, err))
	}
	return nil
}

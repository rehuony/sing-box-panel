import { describe, expect, it, vi } from 'vitest';

import type { ApiClient, CoreArtifact } from '@/api/api-client';

import { installedCoreVersions, listInstalledCoreArtifacts } from '@/utils/installed-core-artifacts';

function artifact(id: string, exactVersion: string, arch: CoreArtifact['arch'] = 'arm64'): CoreArtifact {
  return {
    id,
    exact_version: exactVersion,
    os: 'linux',
    arch,
    variant: 'musl',
    source_kind: 'official',
    repository_id: 509091576,
    release_id: 1,
    asset_id: Number(id.replace(/\D/g, '')) || 1,
    archive_sha256: 'a'.repeat(64),
    binary_sha256: 'b'.repeat(64),
    binary_path: `/artifacts/${id}/sing-box`,
    reported_version: exactVersion,
    feature_fingerprint: { status: 'reported', features: [] },
    created_at: '2026-09-22T00:00:00Z',
  };
}

describe('installed core artifacts', () => {
  it('reads every page and keeps only artifacts for the current platform', async () => {
    const listCoreArtifacts = vi.fn()
      .mockResolvedValueOnce({
        items: [artifact('core_1', '1.9.0'), artifact('core_2', '1.10.0', 'amd64')],
        next: { created_at: '2026-09-21T00:00:00Z', id: 'core_1' },
      })
      .mockResolvedValueOnce({ items: [artifact('core_3', '1.10.0')] });
    const client = { listCoreArtifacts } as unknown as Pick<ApiClient, 'listCoreArtifacts'>;

    await expect(listInstalledCoreArtifacts(
      client,
      { os: 'linux', arch: 'arm64' },
      new AbortController().signal,
    )).resolves.toEqual([artifact('core_1', '1.9.0'), artifact('core_3', '1.10.0')]);
    expect(listCoreArtifacts).toHaveBeenNthCalledWith(2, {
      architecture: 'arm64',
      beforeID: 'core_1',
      beforeTime: '2026-09-21T00:00:00Z',
      limit: 200,
    }, expect.any(AbortSignal));
  });

  it('deduplicates exact versions and sorts by semantic version descending', () => {
    expect(installedCoreVersions([
      artifact('core_1', '1.9.0'),
      artifact('core_2', '2.0.0'),
      artifact('core_3', '1.10.0'),
      artifact('core_4', '1.10.0'),
    ])).toEqual(['2.0.0', '1.10.0', '1.9.0']);
  });
});

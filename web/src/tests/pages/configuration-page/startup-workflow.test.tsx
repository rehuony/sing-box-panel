import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';

import { ApiClientProvider } from '@/api/api-client-context';
import { StartupWorkflow } from '@/pages/configuration-page/startup-workflow';
import {
  createMockApiClient,
  testManualArtifact,
  testRevision,
  testTask,
} from '@/tests/api/mock-api-client';

describe('startupWorkflow', () => {
  it('requires an explicit decision for every manual reattach conflict', async () => {
    const user = userEvent.setup();
    const stale = { ...testManualArtifact, state: 'stale' as const };
    const preview = {
      evidence: {
        startup_artifact_id: stale.id,
        config_sha256: stale.config_sha256,
        base_revision_id: 'revision_40',
        base_revision_sha256: '3'.repeat(64),
        current_head_id: testRevision.id,
        current_head_sha256: testRevision.sha256,
        exact_core_version: '1.13.19',
        core_artifact_id: 'core_1',
        capability: {
          repository: 'rehuony/sing-box-panel',
          commit_sha: '4'.repeat(40),
          manifest_sha256: '5'.repeat(64),
          support_level: 'native_structured' as const,
        },
      },
      base: { ...testRevision, id: 'revision_40', sequence: 40, sha256: '3'.repeat(64) },
      current: testRevision,
      manual: testRevision.document,
      owned_partial: { global: { mode: 'manual' } },
      merged: testRevision.document,
      residual_paths: [],
      conflicts: [{
        path: '/global/mode',
        base: { present: true, value: 'base' },
        current: { present: true, value: 'current' },
        manual: { present: true, value: 'manual' },
      }],
    };
    const client = createMockApiClient({
      listManualArtifacts: vi.fn().mockResolvedValue({
        resolution: { exact_version: '1.13.19', source: 'explicit' },
        items: [stale],
      }),
      previewManualReattach: vi.fn().mockResolvedValue(preview),
      applyManualReattach: vi.fn().mockResolvedValue({
        preview,
        revision: testRevision,
        artifact: stale,
        task: { ...testTask, id: 'task_reattach', status: 'queued' },
      }),
    });

    render(
      <ApiClientProvider client={client}>
        <StartupWorkflow
          baseRevision={testRevision.id}
          capability='native_structured'
          exactVersion='1.13.19'
          onCanonicalChange={vi.fn().mockResolvedValue(undefined)}
        />
      </ApiClientProvider>,
    );

    await user.click(await screen.findByRole('button', { name: 'Reattach' }));
    await user.click(screen.getByRole('button', { name: 'Create reattached revision' }));
    expect(await screen.findByText(/Choose current or manual/)).toBeInTheDocument();
    await user.click(screen.getByRole('radio', { name: /Manual/ }));
    await user.click(screen.getByRole('button', { name: 'Create reattached revision' }));

    expect(client.applyManualReattach).toHaveBeenCalledWith(stale.id, {
      evidence: preview.evidence,
      decisions: { '/global/mode': 'manual' },
    });
  });
});

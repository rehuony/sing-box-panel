import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen, within } from '@testing-library/react';

import { ApiClientProvider } from '@/api/api-client-context';
import { StartupWorkflow } from '@/pages/configuration-page/startup-workflow';
import { createMockApiClient, testArtifacts, testRevision, testStartupArtifact, testTask } from '@/tests/api/mock-api-client';

function renderStartupWorkflow(
  client: ReturnType<typeof createMockApiClient>,
  mutationsDisabled = false,
) {
  render(
    <MemoryRouter>
      <ApiClientProvider client={client}>
        <StartupWorkflow exactVersion='1.13.19' mutationsDisabled={mutationsDisabled} />
      </ApiClientProvider>
    </MemoryRouter>,
  );
}

describe('startupWorkflow', () => {
  it('keeps start, stop and restart controls owned by the telemetry banner', async () => {
    const client = createMockApiClient();
    renderStartupWorkflow(client);

    await screen.findByRole('heading', { name: 'Immutable startup artifact evidence' });
    expect(screen.queryByRole('button', { name: 'Start' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Stop' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Restart' })).not.toBeInTheDocument();
    expect(client.startRuntime).not.toHaveBeenCalled();
    expect(client.stopRuntime).not.toHaveBeenCalled();
    expect(client.restartRuntime).not.toHaveBeenCalled();
  });

  it('shows immutable activation lifecycle evidence in the deploy workbench', async () => {
    const client = createMockApiClient();

    renderStartupWorkflow(client);

    expect(await screen.findByRole('heading', { name: 'Activation history' })).toBeInTheDocument();
    expect(await screen.findByText('bundle_18')).toBeInTheDocument();
    expect(screen.getByText('apply_succeeded')).toBeInTheDocument();
    expect(client.getRuntimeHistory).toHaveBeenCalledWith(
      { limit: 100 },
      expect.any(AbortSignal),
    );
  });

  it('appends older startup artifact evidence with its paired cursor', async () => {
    const user = userEvent.setup();
    const older = {
      ...testStartupArtifact,
      id: 'startup_older',
      created_at: '2026-08-25T07:30:00Z',
      state: 'failed' as const,
    };
    const listStartupArtifacts = vi.fn()
      .mockResolvedValueOnce({
        items: [testStartupArtifact],
        next: { created_at: testStartupArtifact.created_at, id: testStartupArtifact.id },
      })
      .mockResolvedValueOnce({ items: [older] });
    const client = createMockApiClient({ listStartupArtifacts });

    renderStartupWorkflow(client);

    await user.click(await screen.findByRole('button', { name: 'Load older artifacts' }));
    expect(await screen.findByText(older.id)).toBeInTheDocument();
    expect(screen.getByText(testStartupArtifact.id)).toBeInTheDocument();
    expect(listStartupArtifacts).toHaveBeenLastCalledWith({
      beforeID: testStartupArtifact.id,
      beforeTime: testStartupArtifact.created_at,
      coreArtifactID: 'core_1',
      limit: 100,
    }, undefined);
  });

  it('binds apply confirmation to the immutable startup artifact ID', async () => {
    const user = userEvent.setup();
    const activateStartupArtifact = vi.fn().mockResolvedValue({
      activation: {
        startup_artifact_id: testStartupArtifact.id,
        canonical_revision_id: testRevision.id,
        exact_core_version: '1.13.19', core_artifact_id: 'core_1',
        config_sha256: testStartupArtifact.config_sha256,
        activation_bundle_id: 'bundle_19', activation_sha256: '2'.repeat(64),
        monitoring_tier: 'process_only',
      },
      task: { ...testTask, id: 'task_apply', kind: 'runtime-apply', status: 'succeeded' },
    });
    const client = createMockApiClient({ activateStartupArtifact });

    renderStartupWorkflow(client);

    const candidate = await screen.findByLabelText('Ready startup candidate');
    await screen.findByRole('option', { name: /startup_1/ });
    await user.selectOptions(candidate, testStartupArtifact.id);
    await user.click(screen.getByRole('button', { name: 'Apply ready candidate' }));
    const confirmation = await screen.findByRole('alertdialog');
    expect(within(confirmation).getByText(
      `Queue activation for the immutable startup artifact ${testStartupArtifact.id}. Its core, canonical revision and configuration digest are shown above.`,
    )).toBeInTheDocument();
    await user.click(within(confirmation).getByRole('button', { name: `Apply ${testStartupArtifact.id}` }));

    expect(activateStartupArtifact).toHaveBeenCalledWith(testStartupArtifact.id, 'process_only');
    const acceptedNotice = (await screen.findByText(/Activation bundle_19 was accepted as task task_apply/)).closest<HTMLElement>('[role="status"]');
    expect(acceptedNotice).not.toBeNull();
    expect(within(acceptedNotice!).getByRole('link', { name: 'Open task queue' })).toHaveAttribute('href', '/tasks');
    expect(within(acceptedNotice!).queryByText(/queued|running|completed|succeeded/)).not.toBeInTheDocument();
  });

  it('requires typing the exact immutable rollback bundle ID', async () => {
    const user = userEvent.setup();
    const rollbackRuntime = vi.fn().mockResolvedValue({ ...testTask, status: 'queued' });
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue({
        desired_running: true,
        target_generation: 8,
        observation_state: 'running',
        rollback_bundle_id: 'bundle_17',
      }),
      rollbackRuntime,
    });

    renderStartupWorkflow(client);

    await user.click(await screen.findByRole('button', { name: 'Prepare rollback' }));
    const rollback = await screen.findByRole('button', { name: 'Queue rollback' });
    expect(rollback).toBeDisabled();
    await user.type(screen.getByLabelText('Type the rollback bundle ID to confirm'), 'bundle_17');
    expect(rollback).toBeEnabled();
    await user.click(rollback);

    expect(rollbackRuntime).toHaveBeenCalledWith('bundle_17');
    const acceptedNotice = (await screen.findByText(
      /Rollback to bundle bundle_17 was accepted as task/,
    )).closest<HTMLElement>('[role="status"]');
    expect(acceptedNotice).not.toBeNull();
    expect(within(acceptedNotice!).getByRole('link', { name: 'Open task queue' })).toHaveAttribute('href', '/tasks');
  });

  it('allows a JSON-only core to preview and compile the raw configuration', async () => {
    const user = userEvent.setup();
    const compileConfiguration = vi.fn().mockRejectedValue(new Error('test stops after request evidence'));
    const client = createMockApiClient({
      previewConfiguration: vi.fn().mockResolvedValue({
        canonical_revision: testRevision,
        core_artifact: testArtifacts.items[0],
        support: {
          structured: false,
          exact_version: '1.13.19',
          reason: 'This version does not provide a native configuration Schema.',
        },
        config: { log: { level: 'info' } },
      }),
      compileConfiguration,
    });

    renderStartupWorkflow(client);

    const compile = await screen.findByRole('button', { name: 'Compile and queue check' });
    expect(compile).toBeEnabled();
    expect(screen.getByText((_, element) =>
      element?.tagName === 'PRE' && element.textContent?.includes('"level": "info"') === true,
    )).toBeInTheDocument();
    expect(client.getConfigurationSupport).not.toHaveBeenCalled();
    await user.click(compile);

    expect(compileConfiguration).toHaveBeenCalledWith({
      coreArtifactID: 'core_1',
    });
  });

  it('locks every deploy mutation when the canonical draft is not safe to mutate', async () => {
    const pendingArtifact = { ...testStartupArtifact, id: 'startup_pending', state: 'pending' as const };
    const client = createMockApiClient({
      listStartupArtifacts: vi.fn().mockResolvedValue({
        items: [testStartupArtifact, pendingArtifact],
      }),
    });

    renderStartupWorkflow(client, true);

    expect(await screen.findByRole('button', { name: 'Compile and queue check' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Apply ready candidate' })).toBeDisabled();
    expect(await screen.findByRole('button', { name: `Queue check for ${pendingArtifact.id}` })).toBeDisabled();
    expect(client.compileConfiguration).not.toHaveBeenCalled();
    expect(client.activateStartupArtifact).not.toHaveBeenCalled();
    expect(client.checkStartupArtifact).not.toHaveBeenCalled();
  });

  it('cannot apply a candidate from the previously selected core', async () => {
    const user = userEvent.setup();
    const secondCore = { ...testArtifacts.items[0], id: 'core_2' };
    const activateStartupArtifact = vi.fn();
    const client = createMockApiClient({
      activateStartupArtifact,
      listCoreArtifacts: vi.fn().mockResolvedValue({ items: [...testArtifacts.items, secondCore] }),
      listStartupArtifacts: vi.fn().mockImplementation(async (filter) => ({
        items: filter.coreArtifactID === 'core_1' ? [testStartupArtifact] : [],
      })),
    });

    renderStartupWorkflow(client);

    const candidate = await screen.findByLabelText('Ready startup candidate');
    await screen.findByRole('option', { name: /startup_1/ });
    await user.selectOptions(candidate, testStartupArtifact.id);
    expect(screen.getByRole('button', { name: 'Apply ready candidate' })).toBeEnabled();

    await user.selectOptions(screen.getByLabelText('Verified core artifact'), secondCore.id);
    const apply = screen.getByRole('button', { name: 'Apply ready candidate' });
    expect(apply).toBeDisabled();
    await user.click(apply);
    expect(activateStartupArtifact).not.toHaveBeenCalled();
  });
});

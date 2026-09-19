import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';

import type { ApiClient } from '@/api/api-client';

import { CoresPage } from '@/pages/cores-page/cores-page';
import { ApiClientProvider } from '@/api/api-client-context';
import { ControlPlaneContext } from '@/stores/control-plane.store';
import {
  createMockApiClient,
  testArtifacts,
  testDashboardContext,
  testSystemStatus,
  testTask,
} from '@/tests/api/mock-api-client';

function renderCores(client: ApiClient) {
  render(
    <MemoryRouter>
      <ApiClientProvider client={client}>
        <ControlPlaneContext
          value={{
            status: 'ready',
            context: testDashboardContext,
            message: null,
            refresh: vi.fn().mockResolvedValue(undefined),
            setViewVersion: vi.fn(),
            viewVersion: testDashboardContext.view.exactVersion,
          }}
        >
          <CoresPage />
        </ControlPlaneContext>
      </ApiClientProvider>
    </MemoryRouter>,
  );
}
describe('inline version library', () => {
  it('reads the deployed platform and does not allow changing architecture', async () => {
    const client = createMockApiClient();
    renderCores(client);
    expect(await screen.findByText('linux / ARM64')).toBeInTheDocument();
    expect(screen.queryByRole('combobox', { name: 'Architecture' })).not.toBeInTheDocument();
    await waitFor(() =>
      expect(client.listCatalogAssets).toHaveBeenCalledWith(
        { architecture: 'arm64' },
        expect.any(AbortSignal),
      ),
    );
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
  it('queues inline enable and keeps it pending until the real task settles', async () => {
    const user = userEvent.setup();
    let finish!: (task: typeof testTask) => void;
    const queued = { ...testTask, status: 'queued' as const };
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue({
        desired_running: false,
        target_generation: 0,
        observation_state: 'stopped',
      }),
      enableCore: vi.fn().mockImplementation(
        () =>
          new Promise((resolve) => {
            finish = resolve;
          }),
      ),
      getTask: vi.fn().mockResolvedValue({ ...testTask, status: 'succeeded' }),
    });
    renderCores(client);
    await user.click(await screen.findByRole('button', { name: 'Enable' }));
    expect(client.enableCore).toHaveBeenCalledWith('core_1', expect.any(AbortSignal));
    expect(screen.getByRole('button', { name: 'Enable' })).toBeDisabled();
    finish({ ...queued, status: 'succeeded' });
    await waitFor(() => expect(screen.getByRole('button', { name: 'Enable' })).toBeEnabled());
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
  it('stops the running core with the inline disable action and preserves restricted artifacts', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue({
        desired_running: true,
        target_generation: 1,
        observation_state: 'running',
        running: { core_artifact_id: 'core_1' },
      }),
      listCoreArtifacts: vi.fn().mockResolvedValue({
        items: [
          testArtifacts.items[0],
          { ...testArtifacts.items[0], id: 'quarantined', verification_state: 'quarantined' },
        ],
      }),
      stopRuntime: vi.fn().mockResolvedValue({ ...testTask, status: 'succeeded' }),
    });
    renderCores(client);
    await user.click(await screen.findByRole('button', { name: 'Disable' }));
    expect(client.stopRuntime).toHaveBeenCalledWith(expect.any(AbortSignal));
    expect(screen.getByRole('button', { name: 'Enable' })).toBeDisabled();
    expect(client.enableCore).not.toHaveBeenCalled();
  });
  it('paginates all installed versions and keeps evidence inline', async () => {
    const user = userEvent.setup();
    const items = Array.from({ length: 12 }, (_, index) => ({
      ...testArtifacts.items[0],
      id: `core_${index}`,
      exact_version: `1.13.${index}`,
    }));
    renderCores(createMockApiClient({ listCoreArtifacts: vi.fn().mockResolvedValue({ items }) }));
    await screen.findByText('1.13.0');
    await user.selectOptions(screen.getByRole('combobox', { name: 'Items per page' }), '5');
    expect(screen.queryByText('1.13.5')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    const row = screen.getByText('1.13.5').closest('tr')!;
    await user.click(within(row).getByText('Details'));
    expect(within(row).getByText(testArtifacts.items[0].binary_sha256)).toBeVisible();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
  it('never defaults an unknown platform to ARM64', async () => {
    const client = createMockApiClient({
      getSystemStatus: vi.fn().mockResolvedValue({ ...testSystemStatus, platform: undefined }),
    });
    renderCores(client);
    expect(await screen.findByText('Platform unavailable')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Import archive' })).toBeDisabled();
    expect(client.listCatalogAssets).not.toHaveBeenCalled();
  });
});

import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { act, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';

import type { ApiClient } from '@/api/api-client';

import { Toaster } from '@/components/ui/toast';
import { ApiRequestError } from '@/api/api-client';
import { CoresPage } from '@/pages/cores-page/cores-page';
import { ApiClientProvider } from '@/api/api-client-context';
import { ControlPlaneContext } from '@/stores/control-plane.store';
import {
  createMockApiClient,
  testArtifacts,
  testCatalog,
  testDashboardContext,
  testRuntimeStatus,
  testSystemStatus,
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
          <Routes>
            <Route path='/' element={<CoresPage />} />
            <Route path='/configuration' element={<p>Configuration editor</p>} />
          </Routes>
        </ControlPlaneContext>
      </ApiClientProvider>
    </MemoryRouter>,
  );
}
describe('inline version library', () => {
  it('automatically initializes an empty catalog without blocking installed versions', async () => {
    const user = userEvent.setup();
    let finish!: () => void;
    const client = createMockApiClient({
      listCatalogAssets: vi.fn().mockRejectedValueOnce(new Error('Catalog not initialized')).mockResolvedValue(testCatalog),
      refreshCatalog: vi.fn().mockImplementation(() => new Promise(resolve => {
        finish = () => resolve({ refreshed_at: testCatalog.refreshed_at, releases: 1, assets: 1, not_modified: false });
      })),
    });
    renderCores(client);
    expect(await screen.findByText(testArtifacts.items[0].exact_version)).toBeVisible();
    expect(client.refreshCatalog).toHaveBeenCalledWith(false, expect.any(AbortSignal));
    await user.click(screen.getByRole('tab', { name: 'Available' }));
    await act(async () => finish());
    expect(await screen.findByRole('link', { name: testCatalog.assets[0].name })).toBeVisible();
    expect(client.listCatalogAssets).toHaveBeenCalledTimes(2);
  });

  it('uses cached catalog rows until the user requests a forced GitHub check', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      refreshCatalog: vi.fn().mockResolvedValue({
        refreshed_at: testCatalog.refreshed_at, releases: 1, assets: 1, not_modified: false,
      }),
    });
    renderCores(client);
    await screen.findByText(testArtifacts.items[0].exact_version);
    expect(client.refreshCatalog).not.toHaveBeenCalled();
    await user.click(screen.getByRole('tab', { name: 'Available' }));
    expect(await screen.findByRole('link', { name: testCatalog.assets[0].name })).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Check GitHub for updates' }));
    await waitFor(() => expect(client.refreshCatalog).toHaveBeenCalledWith(true, expect.any(AbortSignal)));
  });

  it('refreshes installed records without contacting GitHub', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    renderCores(client);
    await screen.findByText(testArtifacts.items[0].exact_version);
    const initialCalls = client.listCoreArtifacts.mock.calls.length;
    await user.click(screen.getByRole('button', { name: 'Refresh installed versions' }));
    await waitFor(() => expect(client.listCoreArtifacts.mock.calls.length).toBeGreaterThan(initialCalls));
    expect(client.refreshCatalog).not.toHaveBeenCalled();
  });

  it('switches library tabs by keyboard and associates the visible panel with its tab', async () => {
    const user = userEvent.setup();
    renderCores(createMockApiClient());
    await screen.findByText(testArtifacts.items[0].exact_version);
    const installed = screen.getByRole('tab', { name: 'Installed' });
    const available = screen.getByRole('tab', { name: 'Available' });

    await user.click(installed);
    await user.keyboard('[ArrowRight]');
    expect(available).toHaveFocus();
    await user.keyboard('[Enter]');
    expect(available).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('tabpanel', { name: 'Available' })).toHaveAttribute(
      'id', available.getAttribute('aria-controls'),
    );
    expect(screen.queryByRole('button', { name: 'Enable' })).not.toBeInTheDocument();

    await user.keyboard('[ArrowLeft]');
    expect(installed).toHaveFocus();
    await user.keyboard('[Space]');
    expect(installed).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('tabpanel', { name: 'Installed' })).toHaveAttribute(
      'id', installed.getAttribute('aria-controls'),
    );
    expect(screen.getByRole('button', { name: 'Enable' })).toBeVisible();
  });

  it('reads the deployed platform and does not allow changing architecture', async () => {
    const client = createMockApiClient();
    renderCores(client);
    expect(screen.queryByRole('combobox', { name: 'Architecture' })).not.toBeInTheDocument();
    await waitFor(() =>
      expect(client.listCatalogAssets).toHaveBeenCalledWith(
        { architecture: 'arm64' },
        expect.any(AbortSignal),
      ),
    );
    expect(screen.getByRole('button', { name: 'Import archive' })).toBeEnabled();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
  it('keeps inline enable pending until the operation finishes', async () => {
    const user = userEvent.setup();
    let finish!: (status: typeof testRuntimeStatus) => void;
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
    });
    renderCores(client);
    await user.click(await screen.findByRole('button', { name: 'Enable' }));
    expect(client.enableCore).toHaveBeenCalledWith('core_1', expect.any(AbortSignal));
    expect(screen.getByRole('button', { name: 'Enable' })).toBeDisabled();
    finish(testRuntimeStatus);
    await waitFor(() => expect(screen.getByRole('button', { name: 'Enable' })).toBeEnabled());
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('guides first-time enablement to configuration without claiming success', async () => {
    const user = userEvent.setup();
    render(<Toaster />);
    const client = createMockApiClient({
      enableCore: vi.fn().mockRejectedValue(new ApiRequestError('Configuration not saved', {
        status: 409, code: 'configuration_not_saved',
      })),
    });
    renderCores(client);
    await user.click(await screen.findByRole('button', { name: 'Enable' }));
    expect(await screen.findByText('Save a sing-box configuration in Configuration before enabling or checking a core.')).toBeVisible();
    expect(screen.getByRole('button', { name: 'Enable' })).toBeEnabled();
    expect(screen.queryByRole('button', { name: 'Disable' })).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Open configuration' }));
    expect(await screen.findByText('Configuration editor')).toBeVisible();
  });
  it.each(['running', 'stopped'])('offers disable for the selected version while %s', async (observationState) => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue({
        desired_running: observationState === 'running', target_generation: 1,
        observation_state: observationState,
        enabled_core: { core_artifact_id: 'core_1', exact_core_version: testArtifacts.items[0].exact_version },
      }),
      listCoreArtifacts: vi.fn().mockResolvedValue({ items: [
        testArtifacts.items[0],
        { ...testArtifacts.items[0], id: 'imported', source_kind: 'user_verified' },
      ] }),
      enableCore: vi.fn().mockResolvedValue(testRuntimeStatus),
      disableCore: vi.fn().mockResolvedValue(testRuntimeStatus),
    });
    renderCores(client);
    const enabled = await screen.findByRole('button', { name: 'Disable' });
    expect(enabled).toBeEnabled();
    expect(within(enabled.closest('tr')!).getByRole('button', { name: 'Remove' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Enable' }));
    expect(client.enableCore).toHaveBeenCalledWith('imported', expect.any(AbortSignal));
    expect(client.stopRuntime).not.toHaveBeenCalled();
    await waitFor(() => expect(enabled).toBeEnabled());
    await user.click(enabled);
    expect(client.disableCore).toHaveBeenCalledWith('core_1', expect.any(AbortSignal));
    expect(client.stopRuntime).not.toHaveBeenCalled();
  });

  it('shows installed catalog assets as installed and offers downloads only for missing assets', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      listCatalogAssets: vi.fn().mockResolvedValue({ ...testCatalog, assets: [
        testCatalog.assets[0],
        { ...testCatalog.assets[0], asset_id: 202, version: '1.14.0' },
      ] }),
    });
    renderCores(client);
    await screen.findByText(testArtifacts.items[0].exact_version);
    await user.click(screen.getByRole('tab', { name: 'Available' }));
    const installed = screen.getByText(testCatalog.assets[0].version).closest('tr')!;
    expect(screen.getAllByRole('columnheader').map((header) => header.textContent)).toEqual(['Version', 'Source', 'Official link', 'Actions']);
    expect(within(installed).getAllByRole('cell')).toHaveLength(4);
    const releaseLink = within(installed).getByRole('link', { name: testCatalog.assets[0].name });
    expect(releaseLink).toHaveAttribute('href', 'https://github.com/SagerNet/sing-box/releases/tag/v1.13.19');
    expect(releaseLink).toHaveAttribute('target', '_blank');
    expect(releaseLink).toHaveAttribute('rel', 'noopener noreferrer');
    expect(within(installed).getByRole('button', { name: 'Installed' })).toBeDisabled();
    expect(within(installed).queryByRole('button', { name: 'Download' })).not.toBeInTheDocument();
    const missing = screen.getByText('1.14.0').closest('tr')!;
    expect(within(missing).getByRole('link')).toHaveAttribute('href', 'https://github.com/SagerNet/sing-box/releases/tag/v1.14.0');
    expect(within(missing).getByRole('button', { name: 'Download' })).toBeEnabled();
  });
  it.each(['missing', 'malformed', 'conflicting'])('allows downloads with %s digest metadata', async (kind) => {
    const user = userEvent.setup();
    const asset = {
      ...testCatalog.assets[0], asset_id: 202, version: '1.13.21',
      has_api_digest: kind !== 'missing', api_digest: kind === 'malformed' ? 'invalid' : 'a'.repeat(64),
      has_catalog_digest: kind === 'conflicting', catalog_digest: 'b'.repeat(64),
    };
    const client = createMockApiClient({
      listCatalogAssets: vi.fn().mockResolvedValue({ ...testCatalog, assets: [asset] }),
    });
    renderCores(client);
    await screen.findByText(testArtifacts.items[0].exact_version);
    await user.click(screen.getByRole('tab', { name: 'Available' }));
    const download = await screen.findByRole('button', { name: 'Download' });
    expect(download).toBeEnabled();
    await user.click(download);
    expect(client.installCore).toHaveBeenCalledWith(202, expect.any(AbortSignal));
  });

  it('paginates versions with concise source and runtime columns', async () => {
    const user = userEvent.setup();
    const items = Array.from({ length: 12 }, (_, index) => ({
      ...testArtifacts.items[0],
      id: `core_${index}`,
      exact_version: `1.13.${index}`,
    }));
    renderCores(createMockApiClient({ listCoreArtifacts: vi.fn().mockResolvedValue({ items }) }));
    await screen.findByText('1.13.0');
    await user.click(screen.getByRole('combobox', { name: 'Items per page' }));
    await user.keyboard('[ArrowDown]');
    await user.click(await screen.findByRole('option', { name: '5 per page' }));
    expect(screen.queryByText('1.13.5')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    const row = screen.getByText('1.13.5').closest('tr')!;
    expect(within(row).getByText('Official download')).toBeVisible();
    expect(within(row).getByText('Disabled')).toBeVisible();
    expect(within(row).queryByText('Details')).not.toBeInTheDocument();
    expect(within(row).queryByText(testArtifacts.items[0].variant)).not.toBeInTheDocument();
    expect(within(row).queryByText(testArtifacts.items[0].binary_sha256)).not.toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('jumps to a page and keeps totals and boundary controls in sync with the page size', async () => {
    const user = userEvent.setup();
    const items = Array.from({ length: 12 }, (_, index) => ({
      ...testArtifacts.items[0], id: `core_${index}`, exact_version: `1.13.${index}`,
    }));
    renderCores(createMockApiClient({ listCoreArtifacts: vi.fn().mockResolvedValue({ items }) }));
    await screen.findByText('1.13.0');
    const page = screen.getByRole('spinbutton', { name: 'Current page' });
    expect(page).toHaveValue(1);
    expect(page).toHaveAttribute('max', '2');
    expect(page).toHaveAccessibleDescription('2 pages in total');
    expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled();
    await user.clear(page);
    await user.type(page, '2{Enter}');
    expect(screen.getByText('1.13.10')).toBeVisible();
    expect(screen.queryByText('1.13.0')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled();
    await user.click(screen.getByRole('combobox', { name: 'Items per page' }));
    await user.click(await screen.findByRole('option', { name: '5 per page' }));
    expect(page).toHaveValue(1);
    expect(page).toHaveAttribute('max', '3');
    expect(page).toHaveAccessibleDescription('3 pages in total');
    expect(screen.getByText('1.13.0')).toBeVisible();
    await user.clear(page);
    await user.type(page, '99');
    await user.tab();
    expect(page).toHaveValue(3);
    expect(screen.getByText('1.13.10')).toBeVisible();
    await user.clear(page);
    await user.type(page, '0{Enter}');
    expect(page).toHaveValue(1);
    await user.clear(page);
    await user.type(page, '2{Escape}');
    await user.tab();
    expect(page).toHaveValue(1);
    await user.clear(page);
    await user.tab();
    expect(page).toHaveValue(1);
  });

  it('resets pagination when returning from a filter, tab or page size change', async () => {
    const user = userEvent.setup();
    const items = Array.from({ length: 12 }, (_, index) => ({
      ...testArtifacts.items[0], id: `core_${index}`, exact_version: `1.13.${index}`,
    }));
    renderCores(createMockApiClient({ listCoreArtifacts: vi.fn().mockResolvedValue({ items }) }));
    await screen.findByText('1.13.0');
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    await user.type(screen.getByRole('textbox', { name: 'Filter artifacts' }), '1.13.0');
    await user.clear(screen.getByRole('textbox', { name: 'Filter artifacts' }));
    expect(screen.getByText('1.13.0')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    await user.click(screen.getByRole('tab', { name: 'Available' }));
    await user.click(screen.getByRole('tab', { name: 'Installed' }));
    expect(screen.getByText('1.13.0')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    for (const size of [5, 10]) {
      await user.click(screen.getByRole('combobox', { name: 'Items per page' }));
      await user.click(await screen.findByRole('option', { name: `${size} per page` }));
      expect(screen.getByText('1.13.0')).toBeVisible();
    }
  });

  it('paginates the available list and disables navigation for an empty search', async () => {
    const user = userEvent.setup();
    const assets = Array.from({ length: 12 }, (_, index) => ({
      ...testCatalog.assets[0], asset_id: index + 2000, version: `1.14.${index}`,
      name: `sing-box-1.14.${index}-linux-arm64-musl.tar.gz`,
    }));
    renderCores(createMockApiClient({ listCatalogAssets: vi.fn().mockResolvedValue({ ...testCatalog, assets }) }));
    await screen.findByText(testArtifacts.items[0].exact_version);
    await user.click(screen.getByRole('tab', { name: 'Available' }));
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    expect(screen.getByText('1.14.10')).toBeVisible();
    expect(screen.queryByText('1.14.0')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Previous page' }));
    expect(screen.getByText('1.14.0')).toBeVisible();
    await user.type(screen.getByRole('textbox', { name: 'Filter artifacts' }), 'missing');
    expect(screen.getByRole('spinbutton', { name: 'Current page' })).toHaveValue(1);
    expect(screen.getByRole('spinbutton', { name: 'Current page' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled();
  });
  it('never defaults an unknown platform to ARM64', async () => {
    const client = createMockApiClient({
      getSystemStatus: vi.fn().mockResolvedValue({ ...testSystemStatus, platform: undefined }),
    });
    renderCores(client);
    await waitFor(() => expect(screen.getByRole('tabpanel')).toHaveAttribute('aria-busy', 'false'));
    expect(screen.getByRole('button', { name: 'Import archive' })).toBeDisabled();
    expect(client.listCatalogAssets).not.toHaveBeenCalled();
  });

  it('does not label stale runtime evidence as disabled', async () => {
    renderCores(createMockApiClient({
      getRuntimeStatus: vi.fn().mockResolvedValue({ observation_state: 'stale' }),
      listCoreArtifacts: vi.fn().mockResolvedValue({ items: [{ ...testArtifacts.items[0], source_kind: 'user_verified' }] }),
    }));
    expect(await screen.findByText('Manual import')).toBeVisible();
    expect(screen.getByText('Status unknown')).toBeVisible();
    expect(screen.queryByText('Disabled')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Enable' })).toBeDisabled();
  });

  it('removes a version through a direct button with confirmation', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    renderCores(client);
    await screen.findByText(testArtifacts.items[0].exact_version);
    await user.click(screen.getByRole('button', { name: 'Remove' }));
    const dialog = await screen.findByRole('dialog');
    expect(client.removeCoreArtifact).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Confirm' }));
    expect(client.removeCoreArtifact).toHaveBeenCalledWith(testArtifacts.items[0].id, expect.any(AbortSignal));
  });
});

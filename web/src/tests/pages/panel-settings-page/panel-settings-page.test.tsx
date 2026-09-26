import userEvent from '@testing-library/user-event';
import { Link, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import { ThemeProvider } from '@/theme';
import { Toaster } from '@/components/ui/toast';
import { TooltipProvider } from '@/components/ui/tooltip';
import '@/i18n';
import { ApiClientProvider } from '@/api/api-client-context';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { demoBackupSettings } from '@/api/demo/demo-panel-backup';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { createDemoFilesystemApi } from '@/api/demo/demo-filesystem';
import { PanelSettingsProvider } from '@/stores/panel-settings-provider';
import { ThemeCycleButton } from '@/components/app-shell/theme-cycle-button';
import { PanelSettingsPage } from '@/pages/panel-settings-page/panel-settings-page';

function setup(client = createMockApiClient(), initialEntry = '/panel') {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <ApiClientProvider client={client}>
        <ThemeProvider>
          <TooltipProvider>
            <Toaster>
              <PanelSettingsProvider>
                <ThemeCycleButton />
                <Link to='/'>Leave settings</Link>
                <Link to='/panel'>Open settings</Link>
                <Routes>
                  <Route path='/panel' element={<PanelSettingsPage />} />
                  <Route path='/' element={<div>Dashboard</div>} />
                </Routes>
              </PanelSettingsProvider>
            </Toaster>
          </TooltipProvider>
        </ThemeProvider>
      </ApiClientProvider>
    </MemoryRouter>,
  );
  return client;
}

describe('panel settings', () => {
  beforeEach(() => window.localStorage.removeItem('sing-box-panel.theme'));

  it('loads and saves server defaults independently of the persisted sidebar choice', async () => {
    window.localStorage.setItem('sing-box-panel.theme', 'dark');
    const user = userEvent.setup();
    const client = setup(createMockApiClient(), '/panel#panel-interface');
    const theme = await screen.findByRole('combobox', { name: 'Theme' });
    expect(theme).toHaveTextContent('System');
    expect(document.documentElement).toHaveAttribute('data-theme', 'dark');
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled();

    await user.click(theme);
    await user.click(await screen.findByRole('option', { name: 'Light' }));
    await user.click(screen.getByRole('button', { name: 'Blue' }));
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Corner radius' }), { target: { value: '8' } });
    expect(document.documentElement).toHaveAttribute('data-theme', 'dark');
    await user.click(screen.getByRole('button', { name: 'Theme: Dark. Switch to Light' }));
    await user.click(screen.getByRole('button', { name: 'Theme: Light. Switch to System' }));
    await user.click(screen.getByRole('tab', { name: 'Service settings' }));
    await user.click(screen.getByRole('tab', { name: 'Interface preferences' }));
    expect(screen.getByRole('combobox', { name: 'Theme' })).toHaveTextContent('Light');
    expect(screen.getByRole('spinbutton', { name: 'Corner radius' })).toHaveValue(8);
    expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#2563EB');
    expect(client.savePanelSettings).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({
      preferences: expect.objectContaining({ appearance: { theme: 'light', color: '#2563EB', radius: 8 } }),
    })));
    await waitFor(() => expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled());
    expect(screen.getByRole('combobox', { name: 'Theme' })).toHaveTextContent('Light');
    expect(document.documentElement).toHaveAttribute('data-theme', 'system');
    expect(window.localStorage.getItem('sing-box-panel.theme')).toBe('system');
  });

  it('resets appearance without replacing the traffic draft', async () => {
    const user = userEvent.setup();
    setup();
    await user.click(await screen.findByRole('tab', { name: 'Traffic usage' }));
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Total traffic quota' }), { target: { value: '800' } });
    await user.click(screen.getByRole('tab', { name: 'Interface preferences' }));
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Corner radius' }), { target: { value: '24' } });
    await user.click(screen.getByRole('button', { name: 'Reset defaults' }));
    expect(screen.getByRole('spinbutton', { name: 'Corner radius' })).toHaveValue(12);
    await user.click(screen.getByRole('tab', { name: 'Traffic usage' }));
    expect(screen.getByRole('spinbutton', { name: 'Total traffic quota' })).toHaveValue(800);
  });

  it('masks a configured GitHub token without submitting the display mask as a replacement', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    const view = await client.getPanelSettings();
    vi.mocked(client.getPanelSettings).mockResolvedValue({ ...view, github_token_configured: true });
    setup(client);
    const management = await screen.findByLabelText('Management token', { selector: 'input' });
    await user.click(screen.getByRole('tab', { name: 'System maintenance' }));
    const github = screen.getByLabelText('GitHub Token', { selector: 'input' });
    expect(github).toHaveAttribute('placeholder', (management as HTMLInputElement).value);
    expect(github).toHaveValue('');
    expect(github).toHaveAccessibleDescription('Configured; leave blank to retain');
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Remove' }));
    expect(github).toBeDisabled();
    expect(github).toHaveAttribute('placeholder', 'Removed when saved');
    await user.click(screen.getByRole('button', { name: 'Undo' }));
    expect(github).toBeEnabled();
    expect(github).toHaveAttribute('placeholder', (management as HTMLInputElement).value);
    fireEvent.change(screen.getByLabelText('Version check interval (hours)'), { target: { value: '24' } });
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledWith(expect.objectContaining({
      github_token: '', clear_github_token: false,
    })));
  });

  it('selects the data directory through the shared picker without saving settings', async () => {
    const user = userEvent.setup();
    const client = setup(createMockApiClient(createDemoFilesystemApi()));
    await user.click(await screen.findByRole('tab', { name: 'System maintenance' }));
    await user.click(screen.getByRole('button', { name: 'Browse server path: Data directory' }));
    const dialog = within(screen.getByRole('dialog'));
    await user.click(dialog.getByRole('button', { name: 'Edit path' }));
    fireEvent.change(dialog.getByLabelText('Location'), { target: { value: '/etc/sing-box' } });
    await user.click(dialog.getByRole('button', { name: 'Go' }));
    await waitFor(() => expect(dialog.queryByText('Loading directory…')).not.toBeInTheDocument());
    await user.click(dialog.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(screen.getByLabelText('Data directory', { selector: 'input' })).toHaveValue('/etc/sing-box'));
    expect(client.savePanelSettings).not.toHaveBeenCalled();
  });

  it('focuses the invalid field when save returns to its category', async () => {
    const user = userEvent.setup();
    const client = setup();
    const address = await screen.findByLabelText('Published node address', { selector: 'input' });
    fireEvent.change(address, { target: { value: 'https://invalid.example.com' } });
    await user.click(screen.getByRole('tab', { name: 'Log management' }));
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(screen.getByLabelText('Published node address', { selector: 'input' })).toHaveFocus());
    expect(client.savePanelSettings).not.toHaveBeenCalled();
  });

  it('previews a backup, requires confirmation and submits both destination revisions', async () => {
    const user = userEvent.setup();
    const client = setup();
    const view = await client.getPanelSettings();
    const configuration = await client.getConfigurationFile();
    const native = demoBackupSettings(view, { github: '', management: 'backup-secret' });
    native.data_dir = '/srv/other-machine';
    const backup = {
      format: 'sing-box-panel-backup', version: 1, exported_at: '2026-09-23T01:00:00Z',
      panel_settings: native, sing_box_configuration: '  { unfinished raw text',
    };
    vi.mocked(client.restorePanelBackup).mockResolvedValue({
      settings: { ...view, revision: view.revision + 1 }, reauthentication_required: false,
    });
    await user.click(await screen.findByRole('tab', { name: 'Backup and restore' }));
    const file = new File([JSON.stringify(backup)], 'backup.json', { type: 'application/json' });
    Object.defineProperty(file, 'text', { value: async () => JSON.stringify(backup) });
    await user.upload(screen.getByLabelText('Choose backup file', { selector: 'input' }), file);
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog).toHaveTextContent('/srv/other-machine');
    expect(dialog).not.toHaveTextContent('backup-secret');
    expect(client.restorePanelBackup).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Restore configuration' }));
    await waitFor(() => expect(client.restorePanelBackup).toHaveBeenCalledWith({
      backup, settings_revision: view.revision, configuration_revision: configuration.revision,
    }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled();
  });

  it('confirms leaving unsaved settings and retains a saved appearance', async () => {
    const user = userEvent.setup();
    const client = setup();
    await user.click(await screen.findByRole('tab', { name: 'Interface preferences' }));
    await user.click(screen.getByRole('button', { name: 'Blue' }));
    const radius = screen.getByRole('spinbutton', { name: 'Corner radius' });
    expect(radius).toHaveValue(12);
    fireEvent.change(radius, { target: { value: '8' } });
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#2563EB'));
    expect(document.documentElement.style.getPropertyValue('--radius-control')).toBe('4px');
    await user.click(screen.getByRole('tab', { name: 'Service settings' }));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Interface preferences' }));
    expect(screen.getByRole('spinbutton', { name: 'Corner radius' })).toHaveValue(8);
    await user.click(screen.getByRole('link', { name: 'Leave settings' }));
    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#6D4ED1'));
    expect(client.savePanelSettings).not.toHaveBeenCalled();
    await user.click(screen.getByRole('link', { name: 'Open settings' }));
    await user.click(await screen.findByRole('tab', { name: 'Interface preferences' }));
    await user.click(screen.getByRole('button', { name: 'Green' }));
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledWith(expect.objectContaining({
      revision: 0, preferences: expect.objectContaining({ appearance: expect.objectContaining({ color: '#15803D' }) }),
    })));
    await user.click(screen.getByRole('link', { name: 'Leave settings' }));
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#15803D'));
  });
});

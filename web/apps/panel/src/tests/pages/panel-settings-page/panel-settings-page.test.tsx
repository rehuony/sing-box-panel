import userEvent from '@testing-library/user-event';
import { Toaster } from '@shared/components/ui/toast';
import { Link, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { TooltipProvider } from '@shared/components/ui/tooltip';
import { ThemeCycleButton } from '@shared/components/theme-cycle-button';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import { ThemeProvider, useTheme } from '@/theme';
import '@/i18n';
import { ApiRequestError } from '@/api/api-client';
import { ApiClientProvider } from '@/api/api-client-context';
import { backupSettings } from '@/tests/api/backup-settings';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { PanelSettingsProvider } from '@/stores/panel-settings-provider';
import { PanelSettingsPage } from '@/pages/panel-settings-page/panel-settings-page';

function PanelThemeButton() {
  const { cycleTheme, preference } = useTheme();
  return <ThemeCycleButton onCycle={cycleTheme} preference={preference} />;
}

function setup(client = createMockApiClient(), initialEntry = '/settings') {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <ApiClientProvider client={client}>
        <ThemeProvider>
          <TooltipProvider>
            <Toaster>
              <PanelSettingsProvider>
                <PanelThemeButton />
                <Link to='/'>Leave settings</Link>
                <Link to='/settings'>Open settings</Link>
                <Routes>
                  <Route path='/settings' element={<PanelSettingsPage />} />
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
    const client = setup();
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

  it('resets appearance without replacing the traffic draft, and reverts every change', async () => {
    const user = userEvent.setup();
    setup();
    fireEvent.change(await screen.findByRole('spinbutton', { name: 'Traffic metrics' }), { target: { value: '30' } });
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Corner radius' }), { target: { value: '24' } });
    expect(screen.getByText('2 unsaved changes')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Reset defaults' }));
    expect(screen.getByRole('spinbutton', { name: 'Corner radius' })).toHaveValue(12);
    expect(screen.getByRole('spinbutton', { name: 'Traffic metrics' })).toHaveValue(30);
    expect(screen.getByText('1 unsaved change')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Revert changes' }));
    expect(screen.getByRole('spinbutton', { name: 'Traffic metrics' })).toHaveValue(90);
    expect(screen.queryByText(/unsaved change/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled();
  });

  it('masks a configured GitHub token without submitting the display mask as a replacement', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    const view = await client.getPanelSettings();
    vi.mocked(client.getPanelSettings).mockResolvedValue({ ...view, github_token_configured: true });
    setup(client);
    const github = await screen.findByLabelText('GitHub Token', { selector: 'input' });
    expect(github.getAttribute('placeholder')).toMatch(/^•+$/);
    expect(github).toHaveValue('');
    expect(github).toHaveAccessibleDescription('Configured; leave blank to retain');
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Remove' }));
    expect(github).toHaveAttribute('readonly');
    expect(github).toHaveAttribute('placeholder', 'Removed when saved');
    await user.click(screen.getByRole('button', { name: 'Undo' }));
    expect(github).not.toHaveAttribute('readonly');
    expect(github.getAttribute('placeholder')).toMatch(/^•+$/);
    fireEvent.change(screen.getByLabelText('Check interval'), { target: { value: '24' } });
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledWith(expect.objectContaining({
      github_token: '', clear_github_token: false,
    })));
  });

  it('edits the panel data directory as text, since node pickers browse nodes', async () => {
    const client = setup();
    await screen.findByLabelText('Data directory', { selector: 'input' });
    expect(screen.queryByRole('button', { name: 'Browse server path: Data directory' })).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Data directory', { selector: 'input' }), { target: { value: '/etc/sing-box' } });
    expect(screen.getByLabelText('Data directory', { selector: 'input' })).toHaveValue('/etc/sing-box');
    expect(client.savePanelSettings).not.toHaveBeenCalled();
  });

  it('focuses the invalid field when saving', async () => {
    const user = userEvent.setup();
    const client = setup();
    const origin = await screen.findByLabelText('Access origin', { selector: 'input' });
    fireEvent.change(origin, { target: { value: 'ftp://invalid.example.com/path' } });
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(screen.getByLabelText('Access origin', { selector: 'input' })).toHaveFocus());
    expect(client.savePanelSettings).not.toHaveBeenCalled();
  });

  it('previews a backup, requires confirmation and submits the destination revision', async () => {
    const user = userEvent.setup();
    const client = setup();
    const view = await client.getPanelSettings();
    const native = backupSettings(view, { github: '', passwordHash: 'backup-secret' });
    native.data_dir = '/srv/other-machine';
    const backup = {
      format: 'sing-box-panel-backup', version: 3, exported_at: '2026-09-23T01:00:00Z',
      panel_settings: native,
    };
    vi.mocked(client.restorePanelBackup).mockResolvedValue({
      settings: { ...view, revision: view.revision + 1 }, reauthentication_required: false,
    });
    await screen.findByRole('button', { name: 'Export' });
    const file = new File([JSON.stringify(backup)], 'backup.json', { type: 'application/json' });
    Object.defineProperty(file, 'text', { value: async () => JSON.stringify(backup) });
    await user.upload(screen.getByLabelText('Restore…', { selector: 'input' }), file);
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog).toHaveTextContent('/srv/other-machine');
    expect(dialog).not.toHaveTextContent('backup-secret');
    expect(client.restorePanelBackup).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Restore configuration' }));
    await waitFor(() => expect(client.restorePanelBackup).toHaveBeenCalledWith({
      backup, settings_revision: view.revision,
    }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled();
  });

  it('confirms leaving unsaved settings and retains a saved appearance', async () => {
    const user = userEvent.setup();
    const client = setup();
    await user.click(await screen.findByRole('button', { name: 'Blue' }));
    const radius = screen.getByRole('spinbutton', { name: 'Corner radius' });
    expect(radius).toHaveValue(12);
    fireEvent.change(radius, { target: { value: '8' } });
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#2563EB'));
    expect(document.documentElement.style.getPropertyValue('--radius-control')).toBe('4px');
    await user.click(screen.getByRole('link', { name: 'Leave settings' }));
    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#6D4ED1'));
    expect(client.savePanelSettings).not.toHaveBeenCalled();
    await user.click(screen.getByRole('link', { name: 'Open settings' }));
    await user.click(await screen.findByRole('button', { name: 'Green' }));
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledWith(expect.objectContaining({
      revision: 0, preferences: expect.objectContaining({ appearance: expect.objectContaining({ color: '#15803D' }) }),
    })));
    await user.click(screen.getByRole('link', { name: 'Leave settings' }));
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#15803D'));
  });
  it.each([
    { email: ' New@example.com ', password: '', credentials: { email: 'new@example.com' } },
    { email: 'admin@example.com', password: 'new-password-123', credentials: { new_password: 'new-password-123' } },
    { email: 'new@example.com', password: 'new-password-123', credentials: { email: 'new@example.com', new_password: 'new-password-123' } },
  ])('stages account changes until the page save and retains failed drafts: $credentials', async ({ email, password, credentials }) => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    client.savePanelSettings.mockRejectedValue(new ApiRequestError('Settings changed', { status: 412, code: 'panel_settings_conflict' }));
    setup(client);
    const emailInput = await screen.findByLabelText('Email');
    await user.clear(emailInput);
    await user.type(emailInput, email);
    if (password) {
      await user.click(screen.getByRole('button', { name: 'Change password' }));
      await user.type(screen.getByLabelText('Password', { selector: 'input' }), password);
    }
    expect(screen.queryByLabelText('Current password')).not.toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(client.savePanelSettings).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledExactlyOnceWith(
      expect.objectContaining({ credentials }),
    ));
    expect(screen.getByLabelText('Email')).toHaveValue(email);
    if (password) expect(screen.getByLabelText('Password', { selector: 'input' })).toHaveValue(password);
    expect(client.logout).not.toHaveBeenCalled();
    await user.click(screen.getByRole('link', { name: 'Leave settings' }));
    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    await user.click(screen.getByRole('link', { name: 'Open settings' }));
    expect(await screen.findByLabelText('Email')).toHaveValue('admin@example.com');
    expect(screen.queryByLabelText('Password', { selector: 'input' })).not.toBeInTheDocument();
  });
  it.each([
    ['Email', 'invalid', 'admin-email'],
    ['Password', 'short', 'new-password'],
  ])('locates an invalid %s when saving', async (label, value, id) => {
    const user = userEvent.setup();
    const client = setup();
    if (id === 'new-password') await user.click(await screen.findByRole('button', { name: 'Change password' }));
    const input = await screen.findByLabelText(label, { selector: 'input' });
    fireEvent.change(input, { target: { value } });
    await user.click(screen.getByLabelText('Traffic metrics'));
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(document.getElementById(id)).toHaveFocus());
    expect(client.savePanelSettings).not.toHaveBeenCalled();
  });
});

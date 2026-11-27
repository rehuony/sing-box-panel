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

  it('lists the backups here and backs up the categories chosen with a passphrase typed twice', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    const counts = { nodes: 3, sources: 2, channels: 4, rule_sets: 5, keys: 12, history_days: 30, log_entries: 0 };
    vi.mocked(client.listPanelBackups).mockResolvedValue({ items: [
      { name: 'sing-box-panel-20261127-030000-scheduled.tar.gz', kind: 'scheduled', created_at: '2026-11-27T03:00:00Z', size: 2048, encrypted: false,
        categories: ['settings', 'nodes', 'subscriptions', 'rule_sets', 'keys', 'history'], counts },
      { name: 'sing-box-panel-20261126-211400-manual.tar.gz', kind: 'manual', created_at: '2026-11-26T21:14:00Z', size: 4096, encrypted: true,
        categories: ['rule_sets', 'keys'], counts },
    ] });
    vi.mocked(client.createPanelBackup).mockResolvedValue({
      name: 'sing-box-panel-20261127-100000-manual.tar.gz', kind: 'manual', created_at: '2026-11-27T10:00:00Z', size: 4096, encrypted: true,
      categories: ['settings', 'nodes', 'subscriptions', 'rule_sets', 'keys', 'logs'], counts,
    });
    setup(client);
    expect(await screen.findByText('2 backups')).toBeVisible();
    expect(screen.getAllByRole('listitem')).toHaveLength(2);
    expect(screen.getByRole('img', { name: 'Encrypted' })).toBeVisible();
    expect(screen.getByText('Manual · Rule sets, Keys and users')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'Back up now' }));
    const dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByRole('checkbox', { name: 'Logs' })).not.toBeChecked();
    await user.click(within(dialog).getByRole('checkbox', { name: 'Logs' }));
    await user.click(within(dialog).getByRole('checkbox', { name: 'Traffic and runtime records' }));
    expect(within(dialog).getByRole('status')).toHaveTextContent('Partial backup: Panel settings, Nodes, Sources and channels, Rule sets, Keys and users, Logs');
    await user.type(within(dialog).getByLabelText('Passphrase'), 'correct horse');
    await user.type(within(dialog).getByLabelText('Confirm'), 'correct hors');
    await user.click(within(dialog).getByRole('button', { name: 'Back up' }));
    // A problem goes to a toast and marks the field at fault.
    expect(await screen.findByText('The passphrases do not match')).toBeVisible();
    expect(within(dialog).getByLabelText('Confirm')).toHaveAttribute('aria-invalid', 'true');
    expect(client.createPanelBackup).not.toHaveBeenCalled();
    await user.type(within(dialog).getByLabelText('Confirm'), 'e');
    await user.click(within(dialog).getByRole('button', { name: 'Back up' }));
    await waitFor(() => expect(client.createPanelBackup).toHaveBeenCalledWith({
      categories: ['settings', 'nodes', 'subscriptions', 'rule_sets', 'keys', 'logs'], passphrase: 'correct horse',
    }));
    // The dialog's submit never reaches the settings form behind it.
    expect(client.savePanelSettings).not.toHaveBeenCalled();
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Back up now' })).not.toBeInTheDocument());
  });

  it('restores the categories chosen of a checked backup, saying what that changes beyond them', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    const counts = { nodes: 3, sources: 2, channels: 4, rule_sets: 5, keys: 12, history_days: 30, log_entries: 0 };
    const categories = ['settings', 'nodes', 'subscriptions', 'rule_sets', 'keys', 'history'] as const;
    const backup = {
      name: 'sing-box-panel-20261127-030000-scheduled.tar.gz', kind: 'scheduled' as const, created_at: '2026-11-27T03:00:00Z', size: 2048,
      encrypted: false, categories: [...categories], counts,
    };
    vi.mocked(client.listPanelBackups).mockResolvedValue({ items: [backup] });
    vi.mocked(client.stagePanelBackup).mockResolvedValue({
      id: '42', same_panel: false, staged_at: '2026-11-27T10:00:00Z',
      manifest: {
        format: 'sing-box-panel-backup', version: 5, created_at: backup.created_at, kind: 'scheduled', panel_version: 'v0.5.5',
        encrypted: false, categories: [...categories], snapshot: { panel_id: 'panel-a', schema_version: 3, counts }, files: [],
      },
    });
    vi.mocked(client.planPanelRestore).mockImplementation(async (_id, input) => ({
      signs_out: input.categories.includes('settings'),
      effects: {
        pool_nodes: 0, rejected_exits: 0, direct_exits: 0, grants: 0, history_nodes: 0,
        exit_rules: input.categories.includes('rule_sets') && !input.categories.includes('subscriptions') ? 2 : 0,
      },
    }));
    vi.mocked(client.applyPanelRestore).mockResolvedValue({ started_at: '2026-11-27T09:00:00Z', before: { ...backup, kind: 'pre_restore' }, restarting: false });
    setup(client);
    await user.click(await screen.findByRole('button', { name: /^More actions for the / }));
    await user.click(await screen.findByRole('menuitem', { name: 'Restore…' }));
    const dialog = await screen.findByRole('dialog');
    await waitFor(() => expect(within(dialog).getByRole('checkbox', { name: /^Rule sets/ })).toBeChecked());
    expect(client.stagePanelBackup).toHaveBeenCalledWith(backup.name, {}, expect.any(AbortSignal));
    expect(within(dialog).getByRole('checkbox', { name: /^Logs/ })).toHaveAttribute('aria-disabled', 'true');
    expect(within(dialog).getByRole('status')).toHaveTextContent('Restoring the settings signs everyone out, From another panel; its nodes must enroll again');
    const keptHere = [
      /^Panel settings/, /^Nodes/, /^Sources and channels/, /^Keys and users/, /^Traffic and runtime records/,
    ];
    for (const name of keptHere) {
      await user.click(within(dialog).getByRole('checkbox', { name }));
    }
    await waitFor(() => expect(within(dialog).getByRole('status')).toHaveTextContent(/^2 exit rules removed$/));
    expect(client.applyPanelRestore).not.toHaveBeenCalled();
    await user.click(within(dialog).getByRole('button', { name: 'Restore and restart' }));
    await waitFor(() => expect(client.applyPanelRestore).toHaveBeenCalledWith('42', { take_service: false, categories: ['rule_sets'] }));
    expect(await screen.findByText('The restore applies when the panel next starts')).toBeVisible();
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

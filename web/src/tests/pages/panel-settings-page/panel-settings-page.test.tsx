import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { Link, Route, Routes, useNavigate } from 'react-router-dom';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import { ThemeProvider } from '@/theme';
import { Toaster } from '@/components/ui/toast';
import { TooltipProvider } from '@/components/ui/tooltip';
import '@/i18n';
import { ApiClientProvider } from '@/api/api-client-context';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { demoBackupSettings } from '@/api/demo/demo-panel-backup';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { PanelSettingsProvider } from '@/stores/panel-settings-provider';
import { PanelSettingsPage } from '@/pages/panel-settings-page/panel-settings-page';

function HistoryControls() {
  const navigate = useNavigate();
  return (
    <>
      <button onClick={() => void navigate(-1)}>Back</button>
      <button onClick={() => void navigate(1)}>Forward</button>
    </>
  );
}

function setup(client = createMockApiClient(), initialEntry = '/panel') {
  render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <ApiClientProvider client={client}>
        <ThemeProvider>
          <TooltipProvider>
            <Toaster>
              <PanelSettingsProvider>
                <HistoryControls />
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
  it('shares one draft across topics and preserves hidden settings', async () => {
    const user = userEvent.setup();
    const client = setup();
    const original = await client.getPanelSettings();
    await screen.findByLabelText('Base path', { selector: 'input' });
    expect(screen.getAllByRole('tab').map(tab => tab.textContent)).toEqual([
      'Service settings', 'Traffic usage', 'Log management', 'Interface preferences', 'System maintenance', 'Backup and restore',
    ]);
    fireEvent.change(screen.getByLabelText('Base path', { selector: 'input' }), { target: { value: '/control' } });
    fireEvent.change(screen.getByLabelText('Access origin', { selector: 'input' }), { target: { value: 'https://panel.example.com' } });
    expect(screen.getByRole('switch', { name: 'HTTPS-only session cookie' })).toBeChecked();
    fireEvent.change(screen.getByLabelText('Published node address', { selector: 'input' }), { target: { value: 'node.example.com' } });
    await user.click(screen.getByRole('tab', { name: 'System maintenance' }));
    fireEvent.change(screen.getByLabelText('Data directory', { selector: 'input' }), { target: { value: '/srv/panel' } });
    expect(screen.getByLabelText('GitHub Token', { selector: 'input' })).not.toHaveAttribute('placeholder');
    fireEvent.change(screen.getByLabelText('Version check interval (hours)'), { target: { value: '24' } });
    await user.click(screen.getByRole('tab', { name: 'Service settings' }));
    expect(screen.queryByLabelText('Identity key')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Subscription author')).not.toBeInTheDocument();
    await user.click(screen.getByRole('tab', { name: 'Interface preferences' }));
    expect(screen.getByRole('combobox', { name: 'Display language' })).toBeVisible();
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Corner radius' }), { target: { value: '8' } });
    await user.click(screen.getByRole('tab', { name: 'Traffic usage' }));
    fireEvent.change(screen.getByLabelText('Traffic period (months)'), { target: { value: '3' } });
    fireEvent.change(screen.getByLabelText('Metric retention (days)'), { target: { value: '180' } });
    await user.click(screen.getByRole('tab', { name: 'Log management' }));
    expect(screen.queryByText('Panel events')).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Log retention (days)')).not.toBeInTheDocument();
    fireEvent.change(screen.getByLabelText('Log file retention (days)', { selector: 'input' }), { target: { value: '30' } });
    fireEvent.change(screen.getByLabelText('Maximum log files', { selector: 'input' }), { target: { value: '20' } });
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledWith(expect.objectContaining({
      preferences: expect.objectContaining({
        identity_name: original.preferences.identity_name, public_node_host: 'node.example.com', appearance: expect.objectContaining({ radius: 8 }),
      }),
      service: expect.objectContaining({
        data_dir: '/srv/panel', base_path: '/control', secure_cookie: true, catalog_refresh_interval_hours: 24,
        traffic_period_months: 3, sample_retention_days: 180, core_log_retention_days: 30, core_log_max_files: 20,
        subscription_author: original.service.subscription_author,
        subscription_provider: original.service.subscription_provider,
        private_source_cidrs: original.service.private_source_cidrs,
      }),
    })));
  });

  it.each([
    ['access', 'Service settings', 'Base path'],
    ['authentication', 'Service settings', 'Management token'],
    ['publication', 'Service settings', 'Published node address'],
    ['storage', 'System maintenance', 'Data directory'],
    ['updates', 'System maintenance', 'GitHub Token'],
    ['languageGroup', 'Interface preferences', 'Display language'],
  ])('opens the merged category for the previous %s hash', async (hash, category, field) => {
    setup(createMockApiClient(), `/panel#panel-${hash}`);
    expect(await screen.findByRole('tab', { name: category })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByLabelText(field, { selector: 'input, button[data-slot="select-trigger"]' })).toBeVisible();
  });

  it.each([
    ['Service settings', 'Published node address', 'https://invalid.example.com'],
    ['System maintenance', 'Version check interval (hours)', '0'],
  ])('returns to %s when a merged section contains invalid settings', async (category, field, value) => {
    const user = userEvent.setup();
    const client = setup();
    await user.click(await screen.findByRole('tab', { name: category }));
    fireEvent.change(screen.getByLabelText(field, { selector: 'input' }), { target: { value } });
    await user.click(screen.getByRole('tab', { name: 'Log management' }));
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    expect(screen.getByRole('tab', { name: category })).toHaveAttribute('aria-selected', 'true');
    await waitFor(() => expect(screen.getByLabelText(field, { selector: 'input' })).toHaveFocus());
    expect(client.savePanelSettings).not.toHaveBeenCalled();
  });

  it('preserves drafts through hash history and focuses validation errors in other topics', async () => {
    const user = userEvent.setup();
    const client = setup();
    await user.click(await screen.findByRole('tab', { name: 'Traffic usage' }));
    fireEvent.change(screen.getByLabelText('Traffic period (months)'), { target: { value: '0' } });
    await user.click(screen.getByRole('tab', { name: 'Interface preferences' }));
    await user.click(screen.getByRole('button', { name: 'Back' }));
    expect(screen.getByLabelText('Traffic period (months)')).toHaveValue(0);
    await user.click(screen.getByRole('button', { name: 'Forward' }));
    expect(screen.getByRole('tab', { name: 'Interface preferences' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    const unload = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(unload);
    expect(unload.defaultPrevented).toBe(true);
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    expect(screen.getByRole('tab', { name: 'Traffic usage' })).toHaveAttribute('aria-selected', 'true');
    await waitFor(() => expect(screen.getByLabelText('Traffic period (months)')).toHaveFocus());
    expect(client.savePanelSettings).not.toHaveBeenCalled();
  });

  it('previews a backup, requires confirmation and submits both destination revisions', async () => {
    const user = userEvent.setup();
    const client = setup();
    const view = await client.getPanelSettings();
    const configuration = await client.getConfigurationFile();
    const native = demoBackupSettings(view, { github: '', management: 'backup-secret', identity: '' });
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

  it('rejects malformed imports before offering restore and warns about unexported drafts', async () => {
    const user = userEvent.setup();
    const client = setup();
    fireEvent.change(await screen.findByLabelText('Base path', { selector: 'input' }), { target: { value: '/draft' } });
    await user.click(screen.getByRole('tab', { name: 'Backup and restore' }));
    expect(screen.getByText(/The export includes saved content only/)).toBeVisible();
    const file = new File(['{}'], 'bad.json', { type: 'application/json' });
    Object.defineProperty(file, 'text', { value: async () => '{}' });
    await user.upload(screen.getByLabelText('Choose backup file', { selector: 'input' }), file);
    expect((await screen.findAllByText(/The backup format is unsupported/))[0]).toBeInTheDocument();
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    expect(client.restorePanelBackup).not.toHaveBeenCalled();
  });

  it('applies a custom color only on confirmation and saves it through panel settings', async () => {
    const user = userEvent.setup();
    const client = setup();
    await user.click(await screen.findByRole('tab', { name: 'Interface preferences' }));
    await user.click(screen.getByRole('button', { name: 'Custom color' }));
    const dialog = within(await screen.findByRole('dialog', { name: 'Custom theme color' }));
    const hex = dialog.getByRole('textbox', { name: 'HEX color' });
    fireEvent.change(hex, { target: { value: '#123' } });
    expect(dialog.getByRole('button', { name: 'Apply color' })).toBeDisabled();
    expect(hex).toHaveAttribute('aria-invalid', 'true');
    fireEvent.change(hex, { target: { value: '#ab246f' } });
    expect(hex).toHaveValue('#AB246F');
    expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#6D4ED1');
    await user.click(dialog.getByRole('button', { name: 'Apply color' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#AB246F');
    expect(client.savePanelSettings).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledWith(expect.objectContaining({
      preferences: expect.objectContaining({ appearance: expect.objectContaining({ color: '#AB246F' }) }),
    })));
  });

  it('supports keyboard color picking and discards canceled or escaped drafts', async () => {
    const user = userEvent.setup();
    setup();
    await user.click(await screen.findByRole('tab', { name: 'Interface preferences' }));
    await user.click(screen.getByRole('button', { name: 'Custom color' }));
    const hue = await screen.findByRole('slider', { name: 'Hue' });
    hue.focus();
    // react-colorful reads the legacy keyCode, which user-event does not populate.
    fireEvent.keyDown(hue, { key: 'ArrowRight', keyCode: 39 });
    expect(screen.getByRole('textbox', { name: 'HEX color' })).not.toHaveValue('#6D4ED1');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled();
    await user.click(screen.getByRole('button', { name: 'Custom color' }));
    const hex = await screen.findByRole('textbox', { name: 'HEX color' });
    expect(hex).toHaveValue('#6D4ED1');
    fireEvent.change(hex, { target: { value: '#123456' } });
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled();
    expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#6D4ED1');
  });

  it('shows shared field help on hover and keyboard activation without changing settings', async () => {
    const user = userEvent.setup();
    const client = setup();
    const help = await screen.findByRole('button', { name: 'Access origin' });
    const hint = /Enter the full origin/;
    expect(screen.queryByText(hint)).not.toBeInTheDocument();
    await user.hover(help);
    expect(await screen.findByText(hint)).toBeVisible();
    expect(screen.getByRole('tooltip')).toHaveTextContent(hint);
    await user.unhover(help);
    await waitFor(() => expect(screen.queryByText(hint)).not.toBeInTheDocument());
    await user.click(screen.getByRole('textbox', { name: 'Access origin' }));
    await user.tab({ shift: true });
    expect(help).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(await screen.findByText(hint)).toBeVisible();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByText(hint)).not.toBeInTheDocument());
    expect(screen.getByRole('button', { name: 'Save settings' })).toBeDisabled();
    expect(client.savePanelSettings).not.toHaveBeenCalled();
  });

  it('rejects ambiguous tokens before saving and measures UTF-8 byte length', async () => {
    const user = userEvent.setup();
    const client = setup();
    await user.click(await screen.findByRole('tab', { name: 'Service settings' }));
    await user.click(screen.getByRole('button', { name: 'Change' }));
    const dialog = within(screen.getByRole('dialog'));
    const token = dialog.getByLabelText('New token', { selector: 'input' });
    const confirm = dialog.getByLabelText('Confirm token', { selector: 'input' });
    const save = dialog.getByRole('button', { name: 'Save settings' });
    for (const value of [`${'x'.repeat(32)} `, `\uFEFF${'x'.repeat(32)}`, 'x'.repeat(8193)]) {
      fireEvent.change(token, { target: { value } });
      fireEvent.change(confirm, { target: { value } });
      expect(token).toHaveAttribute('aria-invalid', 'true');
      expect(save).toBeDisabled();
      await user.click(save);
    }
    expect(client.savePanelSettings).not.toHaveBeenCalled();
    const value = 'é'.repeat(16);
    fireEvent.change(token, { target: { value } });
    fireEvent.change(confirm, { target: { value } });
    expect(save).toBeEnabled();
    await user.click(save);
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledWith(
      expect.objectContaining({ management_token: value }),
    ));
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

  it('does not commit failed saves and resets only color and radius', async () => {
    const user = userEvent.setup();
    const client = setup(createMockApiClient({ savePanelSettings: vi.fn().mockRejectedValue(new Error('Conflict')) }));
    await user.click(await screen.findByRole('tab', { name: 'Traffic usage' }));
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Total traffic quota' }), { target: { value: '800' } });
    await user.click(screen.getByRole('tab', { name: 'Interface preferences' }));
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Corner radius' }), { target: { value: '24' } });
    await user.click(screen.getByRole('button', { name: 'Blue' }));
    await user.click(screen.getByRole('button', { name: 'Reset defaults' }));
    expect(screen.getByRole('spinbutton', { name: 'Corner radius' })).toHaveValue(12);
    await user.click(screen.getByRole('tab', { name: 'Traffic usage' }));
    expect(screen.getByRole('spinbutton', { name: 'Total traffic quota' })).toHaveValue(800);
    await user.click(screen.getByRole('tab', { name: 'Interface preferences' }));
    await user.click(screen.getByRole('button', { name: 'Blue' }));
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledOnce());
    await user.click(screen.getByRole('link', { name: 'Leave settings' }));
    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#6D4ED1'));
  });
});

import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { Link, Route, Routes } from 'react-router-dom';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import { ThemeProvider } from '@/theme';
import '@/i18n';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ApiClientProvider } from '@/api/api-client-context';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { PanelSettingsProvider } from '@/stores/panel-settings-provider';
import { PanelSettingsPage } from '@/pages/panel-settings-page/panel-settings-page';

function setup(client = createMockApiClient()) {
  render(
    <MemoryRouter initialEntries={['/panel']}>
      <ApiClientProvider client={client}>
        <ThemeProvider>
          <TooltipProvider>
            <PanelSettingsProvider>
              <Link to='/'>Leave settings</Link>
              <Link to='/panel'>Open settings</Link>
              <Routes>
                <Route path='/panel' element={<PanelSettingsPage />} />
                <Route path='/' element={<div>Dashboard</div>} />
              </Routes>
            </PanelSettingsProvider>
          </TooltipProvider>
        </ThemeProvider>
      </ApiClientProvider>
    </MemoryRouter>,
  );
  return client;
}

describe('panel settings', () => {
  it('edits the full service configuration and leaves empty credentials without placeholders', async () => {
    const user = userEvent.setup();
    const client = setup();
    const dataDir = await screen.findByLabelText('Data directory', { selector: 'input' });
    const access = screen.getByText('Panel access').closest('fieldset')!;
    const authentication = screen.getByText('Authentication').closest('fieldset')!;
    expect(within(access).getByLabelText('Base path', { selector: 'input' })).toBeInTheDocument();
    expect(within(access).queryByRole('switch', { name: 'HTTPS-only session cookie' })).not.toBeInTheDocument();
    expect(within(authentication).getByRole('switch', { name: 'HTTPS-only session cookie' })).toBeInTheDocument();
    expect(within(authentication).getByLabelText('Management token', { selector: 'input' })).toBeInTheDocument();
    expect(screen.getByLabelText('GitHub Token', { selector: 'input' })).not.toHaveAttribute('placeholder');
    fireEvent.change(dataDir, { target: { value: '/srv/panel' } });
    fireEvent.change(screen.getByLabelText('Base path', { selector: 'input' }), { target: { value: '/control' } });
    fireEvent.change(screen.getByLabelText('Access origin', { selector: 'input' }), { target: { value: 'https://panel.example.com' } });
    expect(screen.getByRole('switch', { name: 'HTTPS-only session cookie' })).toBeChecked();
    fireEvent.change(screen.getByLabelText('Version check interval (hours)'), { target: { value: '24' } });
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledWith(expect.objectContaining({
      service: expect.objectContaining({ data_dir: '/srv/panel', base_path: '/control', secure_cookie: true, catalog_refresh_interval_hours: 24 }),
    })));
    await user.click(screen.getByRole('tab', { name: 'Nodes & subscriptions' }));
    expect(screen.getByLabelText('Identity key')).not.toHaveAttribute('placeholder');
    fireEvent.change(screen.getByLabelText('Subscription author'), { target: { value: 'Example' } });
    fireEvent.change(screen.getByLabelText('Subscription provider'), { target: { value: 'Custom' } });
    fireEvent.change(screen.getByLabelText('Allowed private source networks', { selector: 'textarea' }), { target: { value: '10.0.0.0/24\nfd00::/64\n' } });
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenLastCalledWith(expect.objectContaining({
      service: expect.objectContaining({ subscription_author: 'Example', subscription_provider: 'Custom', private_source_cidrs: ['10.0.0.0/24', 'fd00::/64'] }),
    })));
    await user.click(screen.getByRole('tab', { name: 'Usage & appearance' }));
    fireEvent.change(screen.getByLabelText('Traffic period (months)'), { target: { value: '3' } });
    fireEvent.change(screen.getByLabelText('Metric retention (days)'), { target: { value: '180' } });
    fireEvent.change(screen.getByLabelText('Log retention (days)'), { target: { value: '30' } });
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenLastCalledWith(expect.objectContaining({
      service: expect.objectContaining({
        traffic_period_months: 3, sample_retention_days: 180, log_retention_days: 30,
      }),
    })));
  });

  it('applies a custom color only on confirmation and saves it through panel settings', async () => {
    const user = userEvent.setup();
    const client = setup();
    await user.click(await screen.findByRole('tab', { name: 'Usage & appearance' }));
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
    await user.click(await screen.findByRole('tab', { name: 'Usage & appearance' }));
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
    await user.click(await screen.findByRole('button', { name: 'Change' }));
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
    await user.click(await screen.findByRole('tab', { name: 'Usage & appearance' }));
    await user.click(screen.getByRole('button', { name: 'Blue' }));
    const radius = screen.getByRole('spinbutton', { name: 'Corner radius' });
    expect(radius).toHaveValue(12);
    fireEvent.change(radius, { target: { value: '8' } });
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#2563EB'));
    expect(document.documentElement.style.getPropertyValue('--radius-control')).toBe('4px');
    await user.click(screen.getByRole('tab', { name: 'Service & security' }));
    await user.click(screen.getByRole('button', { name: 'Keep editing' }));
    expect(screen.getByRole('spinbutton', { name: 'Corner radius' })).toHaveValue(8);
    await user.click(screen.getByRole('link', { name: 'Leave settings' }));
    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#6D4ED1'));
    expect(client.savePanelSettings).not.toHaveBeenCalled();
    await user.click(screen.getByRole('link', { name: 'Open settings' }));
    await user.click(await screen.findByRole('tab', { name: 'Usage & appearance' }));
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
    await user.click(await screen.findByRole('tab', { name: 'Usage & appearance' }));
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Total traffic quota' }), { target: { value: '800' } });
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Corner radius' }), { target: { value: '24' } });
    await user.click(screen.getByRole('button', { name: 'Blue' }));
    await user.click(screen.getByRole('button', { name: 'Reset defaults' }));
    expect(screen.getByRole('spinbutton', { name: 'Corner radius' })).toHaveValue(12);
    expect(screen.getByRole('spinbutton', { name: 'Total traffic quota' })).toHaveValue(800);
    await user.click(screen.getByRole('button', { name: 'Blue' }));
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledOnce());
    await user.click(screen.getByRole('link', { name: 'Leave settings' }));
    await user.click(screen.getByRole('button', { name: 'Discard changes' }));
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#6D4ED1'));
  });
});

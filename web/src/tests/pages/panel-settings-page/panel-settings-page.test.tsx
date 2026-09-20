import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { Link, MemoryRouter, Route, Routes } from 'react-router-dom';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import { ThemeProvider } from '@/theme';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ApiClientProvider } from '@/api/api-client-context';
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

  it('opens help on hover and click, dismisses with Escape, and leaves settings untouched', async () => {
    const user = userEvent.setup();
    const client = setup();
    const help = await screen.findByRole('button', { name: 'Access origin' });
    await user.hover(help);
    const info = await screen.findByRole('dialog', { name: 'Access origin' });
    expect(info).not.toBeEmptyDOMElement();
    await user.unhover(help);
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Access origin' })).not.toBeInTheDocument());
    await user.click(help);
    expect(await screen.findByRole('dialog', { name: 'Access origin' })).toBeInTheDocument();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
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

  it('previews across categories, discards on navigation and retains a saved appearance', async () => {
    const user = userEvent.setup();
    const client = setup();
    await user.click(await screen.findByRole('tab', { name: 'Usage & appearance' }));
    await user.click(screen.getByRole('button', { name: 'Blue' }));
    const radius = screen.getByRole('spinbutton', { name: 'Corner radius' });
    fireEvent.change(radius, { target: { value: '8' } });
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#2563EB'));
    expect(document.documentElement.style.getPropertyValue('--radius-control')).toBe('4px');
    await user.click(screen.getByRole('tab', { name: 'Service & security' }));
    await user.click(screen.getByRole('tab', { name: 'Usage & appearance' }));
    expect(screen.getByRole('spinbutton', { name: 'Corner radius' })).toHaveValue(8);
    await user.click(screen.getByRole('link', { name: 'Leave settings' }));
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
    await user.click(screen.getByRole('button', { name: 'Blue' }));
    await user.click(screen.getByRole('button', { name: 'Reset defaults' }));
    expect(screen.getByRole('spinbutton', { name: 'Total traffic quota' })).toHaveValue(800);
    await user.click(screen.getByRole('button', { name: 'Blue' }));
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await waitFor(() => expect(client.savePanelSettings).toHaveBeenCalledOnce());
    await user.click(screen.getByRole('link', { name: 'Leave settings' }));
    await waitFor(() => expect(document.documentElement.style.getPropertyValue('--appearance-color')).toBe('#6D4ED1'));
  });
});

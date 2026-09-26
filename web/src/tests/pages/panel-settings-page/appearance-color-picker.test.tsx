import userEvent from '@testing-library/user-event';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import '@/i18n';
import { AppearanceColorPicker } from '@/pages/panel-settings-page/appearance-color-picker/appearance-color-picker';

const nonce = document.createElement('meta');
beforeAll(() => {
  // A real page keeps its CSP nonce for the document's entire lifetime.
  // react-colorful caches one stylesheet per document, including between mounts.
  nonce.name = 'sing-box-panel-style-nonce';
  nonce.content = 'panel-color-picker-test-nonce';
  document.head.append(nonce);
});
afterAll(() => nonce.remove());

async function mount() {
  const onChange = vi.fn();
  const user = userEvent.setup();
  render(<AppearanceColorPicker id='color' value='#6D4ED1' onChange={onChange} />);
  await user.click(screen.getByRole('button', { name: 'Custom color' }));
  return { user, onChange };
}

describe('appearance color picker', () => {
  it('authorizes the actual injected stylesheet with the page CSP nonce', async () => {
    await mount();
    const stylesheet = [...document.querySelectorAll('style')]
      .find(style => style.textContent?.includes('.react-colorful__saturation'));
    expect(stylesheet).toBeDefined();
    expect(stylesheet!.nonce).toBe(nonce.content);
  });

  it('applies a valid color only after confirmation', async () => {
    const { user, onChange } = await mount();
    const input = screen.getByRole('textbox', { name: 'HEX color' });
    fireEvent.change(input, { target: { value: '#12' } });
    expect(screen.getByRole('button', { name: 'Apply color' })).toBeDisabled();
    fireEvent.change(input, { target: { value: '#123456' } });
    expect(onChange).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Apply color' }));
    await waitFor(() => expect(onChange).toHaveBeenCalledExactlyOnceWith('#123456'));
  });

  it('supports keyboard color changes and discards canceled or escaped drafts', async () => {
    const { user, onChange } = await mount();
    const hue = screen.getByRole('slider', { name: 'Hue' });
    hue.focus();
    // react-colorful reads keyCode, which user-event does not populate.
    fireEvent.keyDown(hue, { key: 'ArrowRight', keyCode: 39 });
    expect(screen.getByRole('textbox', { name: 'HEX color' })).not.toHaveValue('#6D4ED1');
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    await user.click(screen.getByRole('button', { name: 'Custom color' }));
    const input = screen.getByRole('textbox', { name: 'HEX color' });
    expect(input).toHaveValue('#6D4ED1');
    fireEvent.change(input, { target: { value: '#123456' } });
    await user.keyboard('{Escape}');
    expect(onChange).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Custom color' }));
    expect(screen.getByRole('textbox', { name: 'HEX color' })).toHaveValue('#6D4ED1');
  });
});

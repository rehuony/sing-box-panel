import userEvent from '@testing-library/user-event';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { setAppLanguage } from '@/i18n';
import { LanguageMenu } from '@/components/app-shell/language-menu';

const useReducedMotionMock = vi.hoisted(() => vi.fn(() => false));

vi.mock('motion/react', async (importOriginal) => ({
  ...await importOriginal<typeof import('motion/react')>(),
  useReducedMotion: useReducedMotionMock,
}));

describe('language menu', () => {
  beforeEach(async () => {
    useReducedMotionMock.mockReturnValue(false);
    await setAppLanguage('en');
    window.localStorage.removeItem('sing-box-panel.language');
  });

  afterEach(async () => {
    await setAppLanguage('en');
    window.localStorage.removeItem('sing-box-panel.language');
  });

  it('cross-fades on pointer, keyboard focus and open state, then settles after selection', async () => {
    const user = userEvent.setup();
    const { container } = render(<LanguageMenu />);
    const button = screen.getByRole('button', { name: 'Open language menu' });
    const glyph = container.querySelector('[data-motion="full"]');

    expect(glyph).toHaveAttribute('data-active', 'false');
    await user.hover(button);
    expect(glyph).toHaveAttribute('data-active', 'true');
    await user.unhover(button);
    expect(glyph).toHaveAttribute('data-active', 'false');

    await user.tab();
    expect(button).toHaveFocus();
    expect(glyph).toHaveAttribute('data-active', 'true');

    await user.click(button);
    expect(glyph).toHaveAttribute('data-active', 'true');
    await user.click(await screen.findByRole('menuitemradio', { name: '简体中文' }));

    await waitFor(() => expect(window.localStorage.getItem('sing-box-panel.language')).toBe('zh-CN'));
    expect(glyph).toHaveAttribute('data-active', 'false');
  });

  it('uses one static Languages glyph when reduced motion is requested', () => {
    useReducedMotionMock.mockReturnValue(true);
    const { container } = render(<LanguageMenu />);

    expect(container.querySelector('[data-motion="reduced"]')).toBeInTheDocument();
    expect(container.querySelectorAll('[data-language-icon="languages"]')).toHaveLength(1);
    expect(container.querySelector('[data-language-icon="globe"]')).not.toBeInTheDocument();
  });
});

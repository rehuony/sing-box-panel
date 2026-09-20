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

  it('shows only language choices and restores the globe after selection', async () => {
    const user = userEvent.setup();
    const { container } = render(<LanguageMenu />);
    const button = screen.getByRole('button', { name: 'Open language menu' });
    const glyph = container.querySelector('[data-motion="full"]');

    expect(glyph).toHaveAttribute('data-active', 'false');
    await user.hover(button);
    expect(glyph).toHaveAttribute('data-active', 'false');
    await user.unhover(button);
    expect(glyph).toHaveAttribute('data-active', 'false');

    await user.tab();
    expect(button).toHaveFocus();
    expect(glyph).toHaveAttribute('data-active', 'false');

    await user.click(button);
    expect(await screen.findByRole('menu')).toBeInTheDocument();
    expect(glyph).toHaveAttribute('data-active', 'true');
    expect(screen.queryByText('Language')).not.toBeInTheDocument();
    expect(screen.getAllByRole('menuitemradio')).toHaveLength(2);
    await user.click(await screen.findByRole('menuitemradio', { name: '简体中文' }));

    await waitFor(() => expect(window.localStorage.getItem('sing-box-panel.language')).toBe('zh-CN'));
    expect(glyph).toHaveAttribute('data-active', 'false');
  });

  it.each(['escape', 'outside', 'trigger'] as const)('restores the globe when dismissed with %s', async (dismissal) => {
    const user = userEvent.setup();
    const { container } = render(<LanguageMenu />);
    const button = screen.getByRole('button', { name: 'Open language menu' });
    const glyph = container.querySelector('[data-motion="full"]');

    await user.click(button);
    expect(await screen.findByRole('menu')).toBeInTheDocument();
    expect(glyph).toHaveAttribute('data-active', 'true');

    if (dismissal === 'escape') await user.keyboard('{Escape}');
    else if (dismissal === 'outside') await user.click(document.body);
    else await user.click(button);

    await waitFor(() => expect(button).toHaveAttribute('aria-expanded', 'false'));
    expect(glyph).toHaveAttribute('data-active', 'false');
    await user.hover(button);
    expect(glyph).toHaveAttribute('data-active', 'false');
    expect(window.localStorage.getItem('sing-box-panel.language')).toBeNull();
  });

  it('switches between single static glyphs when reduced motion is requested', async () => {
    useReducedMotionMock.mockReturnValue(true);
    const user = userEvent.setup();
    const { container } = render(<LanguageMenu />);

    expect(container.querySelector('[data-motion="reduced"]')).toBeInTheDocument();
    expect(container.querySelectorAll('[data-language-icon="globe"]')).toHaveLength(1);
    expect(container.querySelector('[data-language-icon="languages"]')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Open language menu' }));
    expect(await screen.findByRole('menu')).toBeInTheDocument();
    expect(container.querySelector('[data-language-icon="globe"]')).not.toBeInTheDocument();
    expect(container.querySelectorAll('[data-language-icon="languages"]')).toHaveLength(1);

    await user.keyboard('{Escape}');
    expect(container.querySelectorAll('[data-language-icon="globe"]')).toHaveLength(1);
    expect(container.querySelector('[data-language-icon="languages"]')).not.toBeInTheDocument();
  });
});

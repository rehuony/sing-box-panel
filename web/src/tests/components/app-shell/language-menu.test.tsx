import userEvent from '@testing-library/user-event';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { setAppLanguage } from '@/i18n';
import { LanguageMenu } from '@/components/app-shell/language-menu';

describe('language menu', () => {
  beforeEach(async () => {
    await setAppLanguage('en');
    window.localStorage.removeItem('sing-box-panel.language');
  });

  afterEach(async () => {
    await setAppLanguage('en');
    window.localStorage.removeItem('sing-box-panel.language');
  });

  it('persists the selected application language', async () => {
    const user = userEvent.setup();
    render(<LanguageMenu />);
    await user.click(screen.getByRole('button', { name: 'Open language menu' }));
    await user.click(await screen.findByRole('menuitemradio', { name: '简体中文' }));

    await waitFor(() => expect(window.localStorage.getItem('sing-box-panel.language')).toBe('zh-CN'));
  });
});

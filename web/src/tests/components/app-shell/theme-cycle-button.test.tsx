import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import { setAppLanguage } from '@/i18n';
import { ThemeProvider } from '@/theme';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ThemeCycleButton } from '@/components/app-shell/theme-cycle-button';

describe('theme cycle button', () => {
  beforeEach(async () => {
    window.localStorage.setItem('sing-box-panel.theme', 'light');
    await setAppLanguage('en');
  });

  afterEach(() => {
    window.localStorage.removeItem('sing-box-panel.theme');
  });

  it('announces the current and next themes while cycling by pointer and keyboard', async () => {
    const user = userEvent.setup();
    render(
      <ThemeProvider>
        <TooltipProvider delay={0}>
          <ThemeCycleButton />
        </TooltipProvider>
      </ThemeProvider>,
    );

    const lightButton = screen.getByRole('button', {
      name: 'Theme: Light. Switch to System',
    });
    expect(lightButton).toHaveAttribute('data-theme-preference', 'light');

    await user.click(lightButton);
    const systemButton = screen.getByRole('button', {
      name: 'Theme: System. Switch to Dark',
    });
    expect(systemButton).toHaveAttribute('data-theme-preference', 'system');

    await user.keyboard('{Enter}');
    expect(screen.getByRole('button', {
      name: 'Theme: Dark. Switch to Light',
    })).toHaveAttribute('data-theme-preference', 'dark');

    await user.keyboard(' ');
    expect(screen.getByRole('button', {
      name: 'Theme: Light. Switch to System',
    })).toHaveAttribute('data-theme-preference', 'light');
  });
});

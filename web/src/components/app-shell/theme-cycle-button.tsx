import { useTranslation } from 'react-i18next';
import { MonitorCog, Moon, Sun } from 'lucide-react';

import type { ThemePreference } from '@/theme';

import { Button } from '@/components/ui/button';
import { nextThemePreference, useTheme } from '@/theme';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';

const themeIcons = {
  dark: Moon,
  light: Sun,
  system: MonitorCog,
} satisfies Record<ThemePreference, typeof Sun>;

export function ThemeCycleButton() {
  const { t } = useTranslation();
  const { cycleTheme, preference } = useTheme();
  const nextPreference = nextThemePreference(preference);
  const Icon = themeIcons[preference];
  const label = t('theme.cycle', {
    current: t(`theme.preference.${preference}`),
    next: t(`theme.preference.${nextPreference}`),
  });

  return (
    <Tooltip>
      <TooltipTrigger
        render={(
          <Button
            aria-label={label}
            className='telemetry-theme-button'
            data-theme-preference={preference}
            onClick={cycleTheme}
            size='icon-sm'
            variant='ghost'
          />
        )}
      >
        <Icon aria-hidden='true' />
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

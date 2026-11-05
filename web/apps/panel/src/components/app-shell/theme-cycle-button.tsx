import type { ThemePreference } from '@shared/theme/theme';

import { useTranslation } from 'react-i18next';
import { MonitorCog, Moon, Sun } from 'lucide-react';
import { Button } from '@shared/components/ui/button';
import { nextThemePreference } from '@shared/theme/theme';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@shared/components/ui/tooltip';

import { useTheme } from '@/theme';

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

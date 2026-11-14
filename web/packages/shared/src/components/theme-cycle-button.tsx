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

const themeIcons = {
  dark: Moon,
  light: Sun,
  system: MonitorCog,
} satisfies Record<ThemePreference, typeof Sun>;

interface ThemeCycleButtonProps {
  onCycle: () => void;
  preference: ThemePreference;
  /** Compact in a row of actions; embedded inside a bar. */
  size?: 'icon-sm' | 'icon-xs';
}

/** Cycles the theme preference through light, system and dark; the application owns the preference. */
export function ThemeCycleButton({ onCycle, preference, size = 'icon-sm' }: ThemeCycleButtonProps) {
  const { t } = useTranslation();
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
            onClick={onCycle}
            size={size}
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

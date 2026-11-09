import type { ReactNode } from 'react';

import { LogOut } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { LanguageMenu } from '@shared/components/language-menu';
import { ThemeCycleButton } from '@shared/components/theme-cycle-button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';

import { useThemePreference } from '@/hooks/use-theme-preference';

const logoSource = `${import.meta.env.BASE_URL}favicon.svg`;

interface ConsoleFrameProps {
  busy?: boolean;
  children: ReactNode;
  onSignOut?: () => void;
}

/** A first row across the window, then the console's view centered below it. */
export function ConsoleFrame({ busy = false, children, onSignOut }: ConsoleFrameProps) {
  const { t } = useTranslation();
  const theme = useThemePreference();
  return (
    <div className='flex min-h-svh flex-col'>
      <header className='flex h-16 shrink-0 items-center gap-3 px-4 sm:px-6'>
        <img alt='' aria-hidden='true' className='size-7 rounded-[var(--radius-control)]' height='28' src={logoSource} width='28' />
        <span className='min-w-0 truncate font-[family-name:var(--font-display)] text-sm font-bold'>Sing-Box Node</span>
        <div className='ms-auto flex shrink-0 items-center gap-1'>
          <ThemeCycleButton onCycle={theme.cycle} preference={theme.preference} />
          <LanguageMenu />
          {onSignOut
            ? (
                <Tooltip>
                  <TooltipTrigger
                    render={(
                      <Button
                        aria-label={t('console.signOut')}
                        onClick={() => {
                          if (!busy) onSignOut();
                        }}
                        size='icon-sm'
                        variant='ghost'
                      />
                    )}
                  >
                    <LogOut aria-hidden='true' />
                  </TooltipTrigger>
                  <TooltipContent>{t('console.signOut')}</TooltipContent>
                </Tooltip>
              )
            : null}
        </div>
      </header>
      {/* The bottom inset matches the first row, so the view sits at the window's center. */}
      <main className='flex min-w-0 flex-1 flex-col items-center justify-center px-4 pb-16 outline-none sm:px-6' id='main-content' tabIndex={-1}>
        {children}
      </main>
    </div>
  );
}

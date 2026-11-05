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
  hostname?: string;
  children: ReactNode;
  onSignOut?: () => void;
}

/** One centered column: the wordmark and the node's hostname, then the console's view. */
export function ConsoleFrame({ busy = false, children, hostname, onSignOut }: ConsoleFrameProps) {
  const { t } = useTranslation();
  const theme = useThemePreference();
  return (
    <div className='mx-auto flex min-h-svh w-full max-w-160 flex-col gap-4 px-4 py-4 sm:py-6'>
      <header className='flex min-h-10 items-center gap-3'>
        <img alt='' aria-hidden='true' className='size-7 rounded-[var(--radius-control)]' height='28' src={logoSource} width='28' />
        <div className='flex min-w-0 flex-1 items-baseline gap-2'>
          <span className='shrink-0 font-[family-name:var(--font-display)] text-sm font-bold'>Sing-Box Node</span>
          {hostname
            ? <span className='truncate font-mono text-xs text-muted-foreground' title={hostname}>{hostname}</span>
            : null}
        </div>
        <div className='flex shrink-0 items-center gap-1'>
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
      <main className='flex min-w-0 flex-1 flex-col gap-4 outline-none' id='main-content' tabIndex={-1}>
        {children}
      </main>
    </div>
  );
}

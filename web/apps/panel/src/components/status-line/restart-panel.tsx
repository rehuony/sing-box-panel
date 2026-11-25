import type { ReactNode } from 'react';

import { useState } from 'react';
import { Power } from 'lucide-react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { useQueryClient } from '@tanstack/react-query';
import { Spinner } from '@shared/components/ui/spinner';
import { DropdownMenuItem } from '@shared/components/ui/dropdown-menu';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@shared/components/ui/alert-dialog';

import { useApiClient } from '@/api/api-client-context';
import { usePanelRestart } from '@/stores/panel-restart.store';

import { announceRestart, restartPanel } from './run-restart';

/** The restart's confirmation. Confirming closes it; the status line then shows the restart until the panel is back. */
export function RestartPanelDialog({ onOpenChange, open }: { onOpenChange: (open: boolean) => void; open: boolean }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  function confirm() {
    onOpenChange(false);
    void restartPanel({ cache, client, t }).then(outcome => announceRestart(outcome, t));
  }
  return (
    <AlertDialog onOpenChange={onOpenChange} open={open}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('shell.restart.title')}</AlertDialogTitle>
          <AlertDialogDescription>{t('shell.restart.description')}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
          <AlertDialogAction onClick={confirm}>{t('shell.restart.confirm')}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/** The desktop line's restart action, named by its tooltip; while a restart runs it does nothing. */
export function RestartPanelAction() {
  const { t } = useTranslation();
  const restarting = usePanelRestart(state => state.restarting);
  const [open, setOpen] = useState(false);
  const label = t('shell.restart.action');
  return (
    <>
      <Tooltip>
        <TooltipTrigger
          render={(
            <Button
              aria-label={label}
              onClick={() => {
                if (!restarting) setOpen(true);
              }}
              size='icon-xs'
              variant='ghost'
            />
          )}
        >
          <Power aria-hidden='true' />
        </TooltipTrigger>
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
      <RestartPanelDialog onOpenChange={setOpen} open={open} />
    </>
  );
}

/** The phone's restart in the more menu; while a restart runs it says so, as the phone line has no readout. */
export function RestartPanelMenuItem({ onSelect }: { onSelect: () => void }) {
  const { t } = useTranslation();
  const restarting = usePanelRestart(state => state.restarting);
  return (
    <DropdownMenuItem
      onClick={() => {
        if (!restarting) onSelect();
      }}
    >
      {restarting ? <Spinner aria-hidden='true' /> : <Power aria-hidden='true' />}
      {t(restarting ? 'shell.restart.running' : 'shell.restart.action')}
    </DropdownMenuItem>
  );
}

/**
 * The readout's slot: the fleet readout, or while the panel restarts a
 * spinner and what is happening in its place. The slot is as wide as the
 * wider of the two at all times, so nothing beside it moves.
 */
export function ReadoutSlot({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const restarting = usePanelRestart(state => state.restarting);
  const text = 'inline-flex h-(--control-height-xs) items-center gap-2 px-2 whitespace-nowrap text-muted-foreground [grid-area:1/1]';
  return (
    <div className='grid flex-none justify-items-end'>
      <div className={cn('[grid-area:1/1]', restarting && 'invisible')}>{children}</div>
      <span aria-hidden='true' className={cn(text, 'invisible')}>
        <span className='size-3.5' />
        {t('shell.restart.running')}
      </span>
      <span className={text} role='status'>
        {restarting
          ? (
              <>
                <Spinner aria-hidden='true' className='size-3.5' />
                {t('shell.restart.running')}
              </>
            )
          : null}
      </span>
    </div>
  );
}

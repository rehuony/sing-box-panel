import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ListChecks, Package, Play, RotateCw, Square, Upload } from 'lucide-react';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@shared/components/ui/dropdown-menu';
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

import type { NodeStatus } from '@/utils/fleet-summary';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { SelectField } from '@/components/select-field';
import { describeRequestError } from '@/components/error-notice';
import { useCommandSession } from '@/stores/command-session.store';

import type { BatchAction } from './run-batch';

import { batchNames, batchWarning, runBatch } from './run-batch';

const actions = [
  { action: 'start', icon: Play },
  { action: 'stop', icon: Square },
  { action: 'restart', icon: RotateCw },
  { action: 'install_core', icon: Package },
  { action: 'update', icon: Upload },
] as const satisfies readonly { action: BatchAction; icon: typeof Play }[];

/** The version Install core installs: one the catalog has, newest first; the dialog cannot confirm without one. */
function CoreVersion({ onChange, value }: { onChange: (version: string) => void; value: string }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const versions = useQuery(queries.catalogVersions(client));
  if (versions.isPending) return <p className='text-sm text-muted-foreground'>{t('command.batch.versions.loading')}</p>;
  if (versions.isError) return <p className='text-sm text-[var(--color-danger-text)]'>{describeRequestError(versions.error)}</p>;
  if (!versions.data?.length) return <p className='text-sm text-[var(--color-danger-text)]'>{t('command.batch.versions.none')}</p>;
  return (
    <SelectField
      aria-label={t('command.batch.versions.label')}
      className='w-full'
      items={[{ label: t('command.batch.versions.choose'), value: '' }, ...versions.data.map(version => ({ label: version, value: version }))]}
      onValueChange={onChange}
      value={value}
    />
  );
}

/**
 * The selection's batch actions: start, stop, restart, install a core and
 * update. Each asks first, naming the nodes and what it interrupts, then runs
 * with its results in the command output.
 */
export function BatchActions({ compact, nodes }: { compact: boolean; nodes: readonly NodeStatus[] }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const running = useCommandSession(state => state.running);
  const [action, setAction] = useState<BatchAction | null>(null);
  const [version, setVersion] = useState('');
  const label = action ? t(`command.batch.action.${action}`) : '';
  const warning = action ? batchWarning(action, t) : null;

  function choose(next: BatchAction) {
    setVersion('');
    setAction(next);
  }

  function confirm() {
    if (action === null || running || (action === 'install_core' && version === '')) return;
    useCommandSession.getState().setOpen(true);
    void runBatch({ action, cache, client, exactVersion: action === 'install_core' ? version : undefined, nodes, t });
    setAction(null);
  }

  return (
    <>
      {compact
        ? (
            <DropdownMenu>
              <DropdownMenuTrigger render={<Button aria-label={t('command.batch.actions')} className='size-11' size='icon' variant='ghost' />}>
                <ListChecks aria-hidden='true' />
              </DropdownMenuTrigger>
              <DropdownMenuContent align='end' side='top' sideOffset={8}>
                <DropdownMenuGroup>
                  {actions.map(({ action: each, icon: Icon }) => (
                    <DropdownMenuItem key={each} onClick={() => choose(each)}>
                      <Icon aria-hidden='true' />
                      {t(`command.batch.action.${each}`)}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          )
        : (
            <div aria-label={t('command.batch.actions')} className='flex items-center gap-0.5' role='group'>
              {actions.map(({ action: each, icon: Icon }) => (
                <Button className='font-[family-name:var(--font-body)]' key={each} onClick={() => choose(each)} size='xs' variant='ghost'>
                  <Icon aria-hidden='true' data-icon='inline-start' />
                  {t(`command.batch.action.${each}`)}
                </Button>
              ))}
            </div>
          )}
      <AlertDialog
        onOpenChange={(open) => {
          if (!open) setAction(null);
        }}
        open={action !== null}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('command.batch.title', { action: label, count: nodes.length })}</AlertDialogTitle>
            <AlertDialogDescription className='wrap-anywhere'>{batchNames(nodes, t)}</AlertDialogDescription>
          </AlertDialogHeader>
          {action === 'install_core' ? <CoreVersion onChange={setVersion} value={version} /> : null}
          {/* Installing interrupts nothing, so its note stays muted; every other action warns. */}
          {warning ? <p className={action === 'install_core' ? 'text-sm text-muted-foreground' : 'text-sm text-[var(--color-warning-text)]'}>{warning}</p> : null}
          {running ? <p className='text-sm text-muted-foreground'>{t('command.batch.busy')}</p> : null}
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={confirm} variant={action === 'stop' ? 'destructive' : 'default'}>
              {t('command.batch.run', { action: label, count: nodes.length })}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

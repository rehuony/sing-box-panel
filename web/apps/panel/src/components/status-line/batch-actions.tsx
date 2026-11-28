import { useState } from 'react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { ListChecks, Package, Play, RotateCw, Square, Upload } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';
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

import { batchNames, batchTargets, batchWarning, runBatch } from './run-batch';

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
 * update. Each acts only on the selected nodes it fits, and one that fits
 * none stays in place, quiet, saying why on hover. Each asks first: the
 * title names the node, or counts the nodes and the description names them,
 * with what the action interrupts and how many selected nodes it leaves out;
 * then it runs with its results in the command output.
 */
export function BatchActions({ compact, nodes }: { compact: boolean; nodes: readonly NodeStatus[] }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  // The action stays after the dialog closes, so its content holds while it fades out.
  const [action, setAction] = useState<BatchAction | null>(null);
  const [open, setOpen] = useState(false);
  const [version, setVersion] = useState('');
  const label = action ? t(`command.batch.action.${action}`) : '';
  const { left, targets } = action ? batchTargets(action, nodes) : { left: 0, targets: [] };
  const warning = action ? batchWarning(action, targets.length, t) : null;
  const single = targets.length === 1 ? targets[0]!.node.name : null;

  function choose(next: BatchAction) {
    if (batchTargets(next, nodes).targets.length === 0) return;
    setVersion('');
    setAction(next);
    setOpen(true);
  }

  function confirm() {
    if (action === null || (action === 'install_core' && version === '')) return;
    if (targets.length === 0) {
      setOpen(false);
      return;
    }
    if (useCommandSession.getState().running) {
      toast.add({ title: t('command.batch.busy'), type: 'warning' });
      return;
    }
    useCommandSession.getState().setOpen(true);
    void runBatch({ action, cache, client, exactVersion: action === 'install_core' ? version : undefined, nodes: targets, t });
    setOpen(false);
  }

  /** Why an action fits none of the selection, or null when it fits some. */
  function unfit(each: BatchAction): string | null {
    if (nodes.length === 0) return t('command.batch.unfit.empty');
    return batchTargets(each, nodes).targets.length === 0 ? t(`command.batch.unfit.${each}`) : null;
  }

  // One node is named in the title, with what the action interrupts below; more nodes are named below the title.
  const description = single === null ? batchNames(targets, t) : warning;
  const notes = [
    single === null && warning ? <p className='text-muted-foreground' key='warning'>{warning}</p> : null,
    left > 0 ? <p className='text-[var(--color-text-faint)]' key='left'>{t('command.batch.left', { count: left })}</p> : null,
  ].filter(Boolean);

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
                    <DropdownMenuItem disabled={unfit(each) !== null} key={each} onClick={() => choose(each)}>
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
              {actions.map(({ action: each, icon: Icon }) => {
                const reason = unfit(each);
                const name = t(`command.batch.action.${each}`);
                // Below 64rem the labels give way to the icons, and the tooltip names the action.
                return (
                  <Tooltip key={each}>
                    <TooltipTrigger
                      render={(
                        <Button
                          aria-disabled={reason !== null || undefined}
                          className={cn(
                            'font-[family-name:var(--font-body)]',
                            reason !== null && 'cursor-default text-[var(--color-text-faint)] hover:bg-transparent hover:text-[var(--color-text-faint)]',
                          )}
                          onClick={() => choose(each)}
                          size='xs'
                          variant='ghost'
                        />
                      )}
                    >
                      <Icon aria-hidden='true' data-icon='inline-start' />
                      <span className='max-lg:sr-only'>{name}</span>
                    </TooltipTrigger>
                    <TooltipContent className={reason === null ? 'lg:hidden' : undefined}>{reason ?? name}</TooltipContent>
                  </Tooltip>
                );
              })}
            </div>
          )}
      <AlertDialog onOpenChange={setOpen} open={open}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className='wrap-anywhere'>
              {single === null
                ? t('command.batch.title', { action: label, count: targets.length })
                : t('command.batch.titleNode', { action: label, name: single })}
            </AlertDialogTitle>
            {description ? <AlertDialogDescription className='wrap-anywhere'>{description}</AlertDialogDescription> : null}
          </AlertDialogHeader>
          {action === 'install_core' || notes.length > 0
            ? (
                <div className='flex flex-col gap-1 text-sm'>
                  {action === 'install_core' ? <div className='mb-2'><CoreVersion onChange={setVersion} value={version} /></div> : null}
                  {notes}
                </div>
              )
            : null}
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <AlertDialogAction onClick={confirm} variant={action === 'stop' ? 'destructive' : 'default'}>
              {label}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

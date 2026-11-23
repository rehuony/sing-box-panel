import type { InfiniteData } from '@tanstack/react-query';

import { useState } from 'react';
import { History } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@shared/components/ui/badge';
import { Button } from '@shared/components/ui/button';
import { formatDateTime } from '@shared/lib/format-time';
import { toast } from '@shared/components/ui/toast-manager';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { useInfiniteQuery, useMutation } from '@tanstack/react-query';
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@shared/components/ui/alert-dialog';

import type { CreatedAtCursor, StartupArtifactPage, StartupArtifactSummary } from '@/api/api-client';

import { ApiRequestError } from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';
import { LoadingState } from '@/components/loading-state';
import { useCurrentNode } from '@/stores/current-node.store';
import { describeRequestError } from '@/components/error-notice';
import { useOptionalSharedTelemetry } from '@/components/node-workspace';

/** A revision as its first characters, enough to tell revisions apart in a list. */
function shortID(value: string): string {
  return value.replace(/^[a-z]+_/, '').slice(0, 8);
}

/**
 * Runs an applied configuration again. The node's own apply chooses limited
 * monitoring when the configuration exposes a protected Clash API and process
 * monitoring otherwise; asking for limited first and falling back when the
 * node refuses it reproduces that choice.
 */
async function activate(client: ReturnType<typeof useApiClient>, artifact: StartupArtifactSummary) {
  try {
    return await client.activateStartupArtifact(artifact.id, 'limited');
  } catch (error) {
    if (error instanceof ApiRequestError && error.code === 'monitoring_tier_unavailable') {
      return client.activateStartupArtifact(artifact.id, 'process_only');
    }
    throw error;
  }
}

/**
 * The node's applied configurations, newest first and twenty at a time, with
 * the one running now marked. Rolling back runs a chosen one again after a
 * danger confirmation that says the core restarts and that the saved text,
 * which the next start loads, stays as it is.
 */
export function AppliedConfigurationsDialog() {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const node = useCurrentNode();
  const runtime = useOptionalSharedTelemetry(state => state.runtimeStatus);
  const [open, setOpen] = useState(false);
  const [target, setTarget] = useState<StartupArtifactSummary | null>(null);
  type Pages = InfiniteData<StartupArtifactPage>;
  const applied = useInfiniteQuery<StartupArtifactPage, Error, Pages, string[], CreatedAtCursor | undefined>({
    enabled: open,
    gcTime: 0,
    getNextPageParam: page => page.next,
    initialPageParam: undefined,
    queryFn: ({ pageParam, signal }) => client.listStartupArtifacts(pageParam, signal),
    queryKey: ['appliedConfigurations', client.nodeID ?? ''],
  });
  const rollback = useMutation({
    mutationFn: (artifact: StartupArtifactSummary) => activate(client, artifact),
    onSuccess: () => {
      toast.add({ title: t('configuration.applied.done', { name: node.name }), type: 'success' });
      setTarget(null);
      setOpen(false);
    },
  });
  const running = runtime?.observation_state === 'running' ? runtime : null;
  const current = (artifact: StartupArtifactSummary) => running !== null
    && artifact.canonical_revision_id === running.loaded_canonical_revision_id
    && artifact.exact_core_version === running.running?.exact_core_version;
  const items = applied.data?.pages.flatMap(page => page.items) ?? [];
  const when = (artifact: StartupArtifactSummary) => formatDateTime(artifact.checked_at ?? artifact.created_at, locale);

  let body;
  if (applied.isPending) {
    body = <LoadingState label={t('configuration.applied.loading')} fullScreen={false} />;
  } else if (applied.isError && items.length === 0) {
    body = (
      <div className='m-auto flex flex-col items-center gap-3 text-center' role='status'>
        <p className='text-sm text-[var(--color-danger-text)]'>{describeRequestError(applied.error)}</p>
        <Button onClick={() => void applied.refetch()} variant='outline'>{t('shell.error.retry')}</Button>
      </div>
    );
  } else if (items.length === 0) {
    body = <p className='m-auto text-sm text-muted-foreground' role='status'>{t('configuration.applied.empty')}</p>;
  } else {
    body = (
      <ScrollArea className='min-h-0 flex-1 rounded-[var(--radius-control)] border'>
        <ul aria-label={t('configuration.applied.title')} className='divide-y'>
          {items.map(artifact => (
            <li className='flex min-h-14 items-center gap-3 px-3 py-2' key={artifact.id}>
              <div className='min-w-0 flex-1'>
                <p className='truncate text-sm'>{when(artifact)}</p>
                <p className='truncate font-[family-name:var(--font-data)] text-xs text-muted-foreground'>
                  {`${artifact.exact_core_version} · ${shortID(artifact.canonical_revision_id)}`}
                </p>
              </div>
              {current(artifact)
                ? <Badge variant='success'>{t('configuration.applied.current')}</Badge>
                : (
                    <Button
                      onClick={() => {
                        rollback.reset();
                        setTarget(artifact);
                      }}
                      size='sm'
                      variant='outline'
                    >
                      {t('configuration.applied.rollback')}
                    </Button>
                  )}
            </li>
          ))}
        </ul>
        {applied.hasNextPage || applied.isFetchNextPageError
          ? (
              <div className='flex flex-col items-center gap-2 border-t p-3'>
                {applied.isFetchNextPageError
                  ? <p className='text-sm text-[var(--color-danger-text)]'>{describeRequestError(applied.error)}</p>
                  : null}
                <Button onClick={() => void applied.fetchNextPage()} size='sm' variant='ghost'>
                  {t(applied.isFetchingNextPage ? 'configuration.applied.loadingMore' : 'configuration.applied.loadMore')}
                </Button>
              </div>
            )
          : null}
      </ScrollArea>
    );
  }

  return (
    <>
      <Button onClick={() => setOpen(true)} size='sm' type='button' variant='flat'>
        <History aria-hidden='true' data-icon='inline-start' />
        <span className='max-sm:sr-only'>{t('configuration.applied.action')}</span>
      </Button>
      <Dialog onOpenChange={setOpen} open={open}>
        <DialogContent className='flex h-[min(36rem,calc(100dvh-2rem))] flex-col overflow-hidden sm:max-w-[36rem]'>
          <DialogHeader>
            <DialogTitle>{t('configuration.applied.title')}</DialogTitle>
            <DialogDescription>{t('configuration.applied.description', { name: node.name })}</DialogDescription>
          </DialogHeader>
          <div className='flex min-h-0 flex-1 flex-col'>{body}</div>
        </DialogContent>
      </Dialog>
      <AlertDialog
        onOpenChange={(next) => {
          if (!next && !rollback.isPending) setTarget(null);
        }}
        open={target !== null}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('configuration.applied.confirmTitle', { name: node.name })}</AlertDialogTitle>
            <AlertDialogDescription>
              {target
                ? t('configuration.applied.confirm', {
                    name: node.name,
                    revision: shortID(target.canonical_revision_id),
                    time: when(target),
                    version: target.exact_core_version,
                  })
                : ''}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {/* A refused rollback says why here; the list behind stays as it was. */}
          <p className='min-h-5 text-sm text-[var(--color-danger-text)] wrap-anywhere' role='alert'>
            {rollback.isError ? describeRequestError(rollback.error) : ''}
          </p>
          <AlertDialogFooter>
            <AlertDialogCancel>{t('common.cancel')}</AlertDialogCancel>
            <Button
              onClick={() => {
                if (target !== null && !rollback.isPending) rollback.mutate(target);
              }}
              variant='destructive'
            >
              {t(rollback.isPending ? 'configuration.applied.rolling' : 'configuration.applied.rollback')}
            </Button>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}

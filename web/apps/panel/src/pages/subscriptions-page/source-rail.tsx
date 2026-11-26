import { useState } from 'react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { formatDateTime } from '@shared/lib/format-time';
import { toast } from '@shared/components/ui/toast-manager';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { FileText, Link2, PencilLine, RefreshCw } from 'lucide-react';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { SubscriptionSource } from '@/api/api-client';

import { ApiRequestError } from '@/api/api-client';
import { IconAction } from '@/components/icon-action';
import { useApiClient } from '@/api/api-client-context';
import { RailCount, RailItem } from '@/components/rail';
import { describeRequestError } from '@/components/error-notice';

import type { SourceDialogMode } from './subscription-source-dialog';

import { contentLabel } from './source-format';
import { attemptReason } from './source-read-issue';
import { useIntervalLabel } from './use-interval-label';
import { SubscriptionSourceDialog } from './subscription-source-dialog';

/**
 * A source in the pool's rail: its kind's icon, its name in full, a warning
 * dot while its last attempt failed and the nodes it adds; its kind, refresh
 * interval, content and failure on hover, and refresh and edit in place of
 * the count.
 */
export function SourceRailItem({ count, current, onEdit, onRefresh, onSelect, refreshing, source }: {
  source: SubscriptionSource;
  /** The pool nodes it adds. */
  count: number;
  current: boolean;
  refreshing: boolean;
  onSelect: () => void;
  onEdit: () => void;
  onRefresh: () => void;
}) {
  const { i18n, t } = useTranslation();
  const intervalLabel = useIntervalLabel();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const failure = source.last_error
    ? t('subscriptions.sources.lastError', { reason: attemptReason(source.last_error, t), time: formatDateTime(source.last_attempt_at, locale) || '—' })
    : '';
  const details = [
    source.kind === 'url' ? source.url ?? '' : t('subscriptions.sources.kind.file'),
    source.kind === 'url' ? intervalLabel(source.refresh_minutes ?? 0) : '',
    formatDateTime(source.content_at, locale),
    contentLabel(source.content_format, undefined, t),
    failure,
  ].filter(Boolean).join(' · ');
  const Icon = source.kind === 'url' ? Link2 : FileText;
  return (
    <RailItem
      actions={(
        <>
          {source.kind === 'url'
            ? (
                <IconAction aria-busy={refreshing || undefined} label={t('subscriptions.sources.refreshNow')} onClick={onRefresh} size='icon-xs'>
                  <RefreshCw aria-hidden='true' className={cn(refreshing && 'motion-safe:animate-spin')} />
                </IconAction>
              )
            : null}
          <IconAction label={t('subscriptions.sources.edit')} onClick={onEdit} size='icon-xs'>
            <PencilLine aria-hidden='true' />
          </IconAction>
        </>
      )}
      current={current}
      end={<RailCount>{count}</RailCount>}
      icon={<Icon />}
      marker={failure
        ? (
            <>
              <span aria-hidden='true' className='size-1.5 flex-none rounded-full bg-[var(--color-warning)]' />
              <span className='sr-only'>{failure}</span>
            </>
          )
        : null}
      name={source.name}
      onClick={onSelect}
      details={<span className='break-all'>{details}</span>}
    />
  );
}

/**
 * The dialogs the rail opens: adding or editing a source, and deleting one
 * after a confirmation that says its nodes leave the pool and channels.
 */
export function SourceDialogs({ mode, onModeChange }: {
  mode: SourceDialogMode | null;
  onModeChange: (mode: SourceDialogMode | null) => void;
}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const [deleting, setDeleting] = useState<SubscriptionSource | null>(null);

  const remove = useMutation({
    mutationFn: (source: SubscriptionSource) => client.deleteSubscriptionSource(source.id, source.updated_at),
    onSuccess: () => {
      setDeleting(null);
      toast.add({ title: t('subscriptions.sources.deleted'), type: 'success' });
    },
    onError: (error) => {
      const stale = error instanceof ApiRequestError && error.status === 412;
      toast.add({ title: stale ? t('subscriptions.sources.deleteStale') : describeRequestError(error), type: stale ? 'warning' : 'error' });
      if (stale) setDeleting(null);
    },
    onSettled: () => cache.invalidateQueries({ queryKey: ['sources'] }),
  });

  return (
    <>
      {mode
        ? (
            <SubscriptionSourceDialog
              mode={mode}
              onClose={() => onModeChange(null)}
              onDelete={(source) => {
                onModeChange(null);
                setDeleting(source);
              }}
            />
          )
        : null}
      <Dialog open={deleting !== null} onOpenChange={(open) => {
        if (!open && !remove.isPending) setDeleting(null);
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('subscriptions.sources.deleteTitle')}</DialogTitle>
            <DialogDescription>
              {deleting ? t('subscriptions.sources.deletePrompt', { count: deleting.node_count, name: deleting.name }) : null}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button onClick={() => setDeleting(null)} variant='outline'>{t('common.cancel')}</Button>
            <Button aria-busy={remove.isPending || undefined} onClick={() => deleting && !remove.isPending && remove.mutate(deleting)} variant='destructive'>
              {t('subscriptions.sources.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

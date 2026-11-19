import type { FormEvent, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { useId, useState } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { toast } from '@shared/components/ui/toast-manager';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { SubscriptionNode, SubscriptionNodeList } from '@/api/api-client';

import { nodePath } from '@/utils/node-path';
import { ApiRequestError } from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';

import { nodeOrigin } from './subscription-node-pool';

interface Message {
  text: string;
  tone: 'danger' | 'warning';
}

/** One reading of the node: its label, then its value. */
function Reading({ children, label }: { children: ReactNode; label: string }) {
  return (
    <div className='contents'>
      <dt className='text-muted-foreground'>{label}</dt>
      <dd className='flex min-w-0 items-center gap-2'>{children}</dd>
    </div>
  );
}

/**
 * A pool node's detail: its display name, the only field that edits, where
 * it comes from, and the outbound channels publish for it. A name another
 * node uses, or a node that changed since it was read, is said in the
 * footer's message slot; the changed node is read again so saving can retry.
 */
export function SubscriptionNodeDialog({ node, onClose }: { node: SubscriptionNode; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const nameId = useId();
  const messageId = useId();
  const [name, setName] = useState(node.display_name);
  const [message, setMessage] = useState<Message | null>(null);
  // The detail carries the credentials, so it is read each time the dialog opens and never kept.
  const detail = useQuery({
    queryKey: ['nodes', 'detail', node.id],
    queryFn: ({ signal }) => client.getSubscriptionNode(node.id, signal),
    gcTime: 0,
    staleTime: 0,
    refetchOnWindowFocus: false,
  });
  const current = detail.data ?? node;
  const dirty = name.trim() !== current.display_name;
  useUnsavedChanges(dirty, onClose);

  const save = useMutation({
    mutationFn: (displayName: string) =>
      client.updateSubscriptionNode(node.id, { display_name: displayName }, current.updated_at),
    onSuccess: (saved) => {
      cache.setQueryData<SubscriptionNodeList>(['nodes'], list => list && {
        ...list,
        items: list.items.map(item => item.id === saved.id
          ? { ...item, display_name: saved.display_name, updated_at: saved.updated_at }
          : item),
      });
      toast.add({ title: t('subscriptions.pool.detail.saved'), type: 'success' });
      onClose();
    },
    onError: (error) => {
      if (error instanceof ApiRequestError && error.status === 409) {
        setMessage({ text: t('subscriptions.pool.detail.nameTaken'), tone: 'danger' });
      } else if (error instanceof ApiRequestError && error.status === 412) {
        setMessage({ text: t('subscriptions.pool.detail.stale'), tone: 'warning' });
        void detail.refetch();
        void cache.invalidateQueries({ queryKey: ['nodes'], exact: true });
      } else {
        setMessage({ text: describeRequestError(error), tone: 'danger' });
      }
    },
  });

  function submit(event: FormEvent) {
    event.preventDefault();
    if (save.isPending) return;
    const trimmed = name.trim();
    if (trimmed === '') {
      setMessage({ text: t('subscriptions.pool.detail.nameRequired'), tone: 'danger' });
      return;
    }
    if (!dirty) {
      onClose();
      return;
    }
    setMessage(null);
    save.mutate(trimmed);
  }

  async function copy(value: string) {
    if (value === '') return;
    try {
      await navigator.clipboard.writeText(value);
      toast.add({ title: t('subscriptions.pool.detail.copied'), type: 'success' });
    } catch {
      toast.add({ title: t('subscriptions.pool.detail.copyFailed'), type: 'error' });
    }
  }

  const { origin } = current;
  const outbound = detail.data ? JSON.stringify(detail.data.outbound, null, 2) : '';
  let block;
  if (detail.data) {
    block = (
      <pre aria-label={t('subscriptions.pool.detail.outbound')} className='max-h-64 min-h-0 overflow-auto px-3 pb-3 font-[family-name:var(--font-data)] text-xs leading-5 select-text' tabIndex={0}>
        {outbound}
      </pre>
    );
  } else if (detail.isPending) {
    block = <div className='flex h-24 items-center justify-center' role='status'><Spinner className='size-4' /></div>;
  } else {
    block = <p className='px-3 pb-3 text-sm text-[var(--color-danger-text)]' role='alert'>{describeRequestError(detail.error)}</p>;
  }

  return (
    <Dialog open onOpenChange={(open) => {
      if (!open && !save.isPending) onClose();
    }}>
      <DialogContent>
        <form className='flex min-w-0 flex-col gap-6' noValidate onSubmit={submit}>
          <DialogHeader>
            <DialogTitle className='truncate'>{current.display_name}</DialogTitle>
            <DialogDescription className='sr-only'>{t('subscriptions.pool.detail.description')}</DialogDescription>
          </DialogHeader>
          <div className='grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-4 gap-y-3'>
            <label className='text-sm text-[var(--color-text-secondary)]' htmlFor={nameId}>{t('subscriptions.pool.detail.name')}</label>
            <Input
              aria-describedby={messageId}
              aria-invalid={message?.tone === 'danger' || undefined}
              autoComplete='off'
              id={nameId}
              maxLength={256}
              onChange={(event) => {
                setName(event.target.value);
                setMessage(null);
              }}
              spellCheck={false}
              value={name}
            />
            <dl className='col-span-full grid grid-cols-subgrid gap-y-2 text-sm'>
              <Reading label={t('subscriptions.pool.detail.origin')}>
                <span className='min-w-0 truncate' title={nodeOrigin(current)}>{nodeOrigin(current)}</span>
                {origin.kind === 'inbound' && origin.node_id && origin.inbound_tag
                  ? (
                      <Button
                        className='ms-auto flex-none'
                        render={<Link to={`${nodePath(origin.node_id, 'configuration')}?inbound=${encodeURIComponent(origin.inbound_tag)}`} />}
                        size='xs'
                        variant='flat'
                      >
                        {t('subscriptions.pool.detail.openConfiguration')}
                      </Button>
                    )
                  : null}
              </Reading>
              <Reading label={t('subscriptions.pool.detail.originName')}>
                <span className='min-w-0 truncate font-[family-name:var(--font-data)]' title={current.origin_name}>{current.origin_name}</span>
              </Reading>
            </dl>
          </div>
          <section aria-label={t('subscriptions.pool.detail.outbound')} className='flex min-w-0 flex-col rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)]'>
            <div className='flex min-w-0 items-center justify-between gap-2 px-3 py-1.5'>
              <span className='text-xs text-muted-foreground'>{t('subscriptions.pool.detail.outbound')}</span>
              <Button className='-me-1.5' onClick={() => void copy(outbound)} size='xs' type='button' variant='flat'>
                {t('subscriptions.pool.detail.copy')}
              </Button>
            </div>
            {block}
          </section>
          <DialogFooter className='flex-nowrap'>
            <p
              className={cn('min-h-5 min-w-0 flex-1 truncate text-sm', message?.tone === 'warning' ? 'text-[var(--color-warning-text)]' : 'text-[var(--color-danger-text)]')}
              id={messageId}
              role='status'
              title={message?.text}
            >
              {message?.text}
            </p>
            <Button onClick={onClose} type='button' variant='outline'>{t('common.cancel')}</Button>
            <Button aria-busy={save.isPending || undefined} type='submit'>{t('subscriptions.pool.detail.save')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

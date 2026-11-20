import type { FormEvent, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { FileCode2, PencilLine } from 'lucide-react';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { useEffect, useId, useRef, useState } from 'react';
import { toast } from '@shared/components/ui/toast-manager';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { SubscriptionNode, SubscriptionNodeList } from '@/api/api-client';

import { nodePath } from '@/utils/node-path';
import { ApiRequestError } from '@/api/api-client';
import { useApiClient } from '@/api/api-client-context';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';

import { nodeAddress } from './subscription-node-pool';

interface Message {
  text: string;
  tone: 'danger' | 'warning';
}

/** A row's label column, then its value or control; stacked below 520px. */
const row = 'grid min-h-12 min-w-0 grid-cols-[8rem_minmax(0,1fr)] items-center gap-x-4 gap-y-1 py-1 max-[520px]:min-h-0 max-[520px]:grid-cols-1 max-[520px]:py-2';
const rowLabel = 'text-sm text-[var(--color-text-secondary)]';
/**
 * Values read in the data face, as on the node cards. These faces are joined as
 * plain text, since class merging reads `text-compact` as a colour.
 */
const dataFace = 'font-[family-name:var(--font-data)] text-compact';
/** The name's face, read or edited; at least 16px on phones, where a smaller field zooms the page. */
const nameFace = 'font-[family-name:var(--font-data)] text-base sm:text-compact';

/** One read-only row of the node: its label, then its value. */
function Reading({ children, label }: { children: ReactNode; label: string }) {
  return (
    <dl className={row}>
      <dt className={rowLabel}>{label}</dt>
      <dd className='flex min-w-0 items-center gap-2'>{children}</dd>
    </dl>
  );
}

/**
 * A pool node's detail: its display name over its protocol and address, the
 * name as the only value that edits, from its edit action, its origin with a
 * fleet node's overview and an inbound's configuration a click away, and the
 * outbound channels publish for it. A name another node uses, or a node that
 * changed since it was read, is said in the footer's message slot; the
 * changed node is read again so saving can retry.
 */
export function SubscriptionNodeDialog({ node, onClose }: { node: SubscriptionNode; onClose: () => void }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const nameId = useId();
  // Opening reads the node, so focus starts on the title; the name turns into a field from its edit action.
  const titleRef = useRef<HTMLHeadingElement>(null);
  const nameRef = useRef<HTMLInputElement>(null);
  const [editing, setEditing] = useState(false);
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
  useEffect(() => {
    if (!editing) return;
    nameRef.current?.focus();
    nameRef.current?.select();
  }, [editing]);

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
  const summary = [current.protocol, nodeAddress(current)].filter(Boolean).join(' · ');
  // The fleet node or source with the node's name there; a fleet node's links to its overview.
  const originLabel = `${origin.kind === 'inbound' ? origin.node_name : origin.source_name} · ${current.origin_name}`;
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
      <DialogContent className='sm:max-w-[40rem]' initialFocus={titleRef}>
        <form className='flex min-w-0 flex-col gap-6' noValidate onSubmit={submit}>
          <DialogHeader className='gap-1'>
            <DialogTitle className='truncate outline-none' ref={titleRef} tabIndex={-1}>{current.display_name}</DialogTitle>
            <DialogDescription className='truncate font-[family-name:var(--font-data)] text-xs text-[var(--color-text-secondary)]' title={summary}>
              {summary}
            </DialogDescription>
          </DialogHeader>
          <div className='flex min-w-0 flex-col divide-y'>
            <div className={row}>
              {editing
                ? <label className={rowLabel} htmlFor={nameId}>{t('subscriptions.pool.detail.name')}</label>
                : <span className={rowLabel}>{t('subscriptions.pool.detail.name')}</span>}
              {editing
                ? (
                    // A terminal line: the prompt hangs in the gap, so the text keeps the place it had at rest.
                    <div className='relative flex min-w-0 items-center'>
                      <span
                        aria-hidden='true'
                        className={`absolute end-full top-1/2 me-[1ch] -translate-y-1/2 ${nameFace} ${message?.tone === 'danger' ? 'text-[var(--color-danger-text)]' : 'text-[var(--color-highlight-yellow)]'}`}
                      >
                        λ
                      </span>
                      <input
                        aria-describedby={messageId}
                        aria-invalid={message?.tone === 'danger' || undefined}
                        autoComplete='off'
                        className={`terminal-input min-w-0 flex-1 bg-transparent p-0 leading-5 text-foreground outline-none ${nameFace}`}
                        data-slot='terminal-input'
                        id={nameId}
                        maxLength={256}
                        onBlur={() => {
                          if (!dirty) setEditing(false);
                        }}
                        onChange={(event) => {
                          setName(event.target.value);
                          setMessage(null);
                        }}
                        ref={nameRef}
                        spellCheck={false}
                        value={name}
                      />
                    </div>
                  )
                : (
                    <div className='flex min-w-0 items-center gap-2'>
                      <span className={`min-w-0 truncate leading-5 ${nameFace}`} title={current.display_name}>{current.display_name}</span>
                      <Button
                        aria-label={t('subscriptions.pool.detail.editName')}
                        className='ms-auto flex-none'
                        onClick={() => setEditing(true)}
                        size='icon-xs'
                        title={t('subscriptions.pool.detail.editName')}
                        type='button'
                        variant='ghost'
                      >
                        <PencilLine aria-hidden='true' />
                      </Button>
                    </div>
                  )}
            </div>
            <Reading label={t('subscriptions.pool.detail.origin')}>
              {origin.kind === 'inbound' && origin.node_id
                ? <Link className={`min-w-0 truncate text-primary underline-offset-3 hover:underline ${dataFace}`} title={originLabel} to={nodePath(origin.node_id)}>{originLabel}</Link>
                : <span className={`min-w-0 truncate ${dataFace}`} title={originLabel}>{originLabel}</span>}
              {origin.kind === 'inbound' && origin.node_id && origin.inbound_tag
                ? (
                    <Button
                      className='ms-auto flex-none'
                      render={<Link to={`${nodePath(origin.node_id, 'configuration')}?inbound=${encodeURIComponent(origin.inbound_tag)}`} />}
                      size='xs'
                      variant='flat'
                    >
                      <FileCode2 aria-hidden='true' data-icon='inline-start' />
                      {t('subscriptions.pool.detail.openConfiguration')}
                    </Button>
                  )
                : null}
            </Reading>
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

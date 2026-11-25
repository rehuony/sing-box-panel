import type { FormEvent } from 'react';

import { useTranslation } from 'react-i18next';
import { CirclePlus, Layers3 } from 'lucide-react';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { useDeferredValue, useId, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@shared/components/ui/select';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { SubscriptionFormat, SubscriptionRuleSet } from '@/api/api-client';

import { queries } from '@/api/queries';
import { ApiRequestError } from '@/api/api-client';
import { compareText } from '@/utils/compare-text';
import { messageLine } from '@/utils/message-line';
import { useApiClient } from '@/api/api-client-context';
import { RegionError } from '@/components/region-state';
import { usePageVisible } from '@/hooks/use-page-visible';
import { Rail, RailCount, RailItem } from '@/components/rail';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { describeRequestError } from '@/components/error-notice';
import { StripDivider, StripSearch } from '@/components/title-strip';
import { useUnsavedChangesContext } from '@/stores/unsaved-changes.store';

import { RuleSetEditor } from './rule-set-editor';
import { SubscriptionContentState } from './subscription-content-state';

const CLIENTS: readonly SubscriptionFormat[] = ['sing-box', 'mihomo', 'loon'];

/** Names a rule set when it is added or renamed; the name is unique among the client's sets. */
function NameDialog({ initial, onClose, onSubmit, taken, title }: {
  title: string;
  initial: string;
  /** Names the client's other sets use, compared ignoring case. */
  taken: readonly string[];
  onSubmit: (name: string) => Promise<void> | void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [name, setName] = useState(initial);
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) {
      setMessage(t('subscriptions.ruleSets.validation.setName'));
      return;
    }
    if (taken.some(each => each.toLowerCase() === trimmed.toLowerCase())) {
      setMessage(t('subscriptions.ruleSets.validation.setNameTaken', { name: trimmed }));
      return;
    }
    setBusy(true);
    try {
      await onSubmit(trimmed);
    } catch (error) {
      setMessage(describeRequestError(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open onOpenChange={open => !open && !busy && onClose()}>
      <DialogContent>
        <form className='flex flex-col gap-4' noValidate onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription className='sr-only'>{title}</DialogDescription>
          </DialogHeader>
          <div className='grid grid-cols-[8rem_minmax(0,1fr)] items-center gap-4 max-[520px]:grid-cols-1 max-[520px]:gap-1'>
            <label className='text-sm text-[var(--color-text-secondary)]' htmlFor={id}>{t('subscriptions.ruleSets.field.setName')}</label>
            <Input
              aria-invalid={Boolean(message) || undefined}
              autoComplete='off'
              id={id}
              maxLength={128}
              onChange={(event) => {
                setMessage('');
                setName(event.target.value);
              }}
              value={name}
            />
          </div>
          <DialogFooter className='flex-nowrap'>
            <p className='min-h-5 min-w-0 flex-1 truncate text-sm text-[var(--color-danger-text)]' role='status' title={messageLine(message) || undefined}>{messageLine(message)}</p>
            <Button onClick={onClose} type='button' variant='outline'>{t('common.cancel')}</Button>
            <Button aria-busy={busy || undefined} type='submit'>{t('subscriptions.ruleSets.apply')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * The rule sets tab: a client picker above the rail of that client's rule
 * sets, and the selected set's editor beside it. The strip searches the
 * list and adds a set to the picked client.
 */
export function RuleSetPanel({ active = true, toolbarTarget }: {
  active?: boolean;
  toolbarTarget?: HTMLElement | null;
}) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const visible = usePageVisible();
  const { confirmNavigation } = useUnsavedChangesContext();
  const enabled = active && visible;
  const list = useQuery({ ...queries.ruleSets(client), enabled });
  const [picked, setPicked] = useState<SubscriptionFormat>('mihomo');
  const [selected, setSelected] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const query = useDeferredValue(search.trim().toLowerCase());
  const [naming, setNaming] = useState<{ kind: 'create' } | { kind: 'rename'; initial: string; apply: (name: string) => void } | null>(null);
  const [deleting, setDeleting] = useState<SubscriptionRuleSet | null>(null);

  const all = useMemo(() => list.data ?? [], [list.data]);
  const counts = useMemo(
    () => new Map(CLIENTS.map(each => [each, all.filter(set => set.client === each).length])),
    [all],
  );
  const sets = useMemo(
    () => all.filter(set => set.client === picked).sort((left, right) => compareText(left.name, right.name)),
    [all, picked],
  );
  const shown = sets.filter(set => query === '' || set.name.toLowerCase().includes(query));
  const ruleSet = sets.find(set => set.id === selected) ?? shown[0] ?? null;

  const create = useMutation({
    mutationFn: (name: string) => client.createSubscriptionRuleSet({ client: picked, name, rules: [] }),
    onSuccess: (created) => {
      cache.setQueryData<SubscriptionRuleSet[]>(['ruleSets'], current => [...current ?? [], created]);
      void cache.invalidateQueries({ queryKey: ['ruleSets'] });
      setSelected(created.id);
      setNaming(null);
    },
  });
  const remove = useMutation({
    mutationFn: (set: SubscriptionRuleSet) => client.deleteSubscriptionRuleSet(set.id, set.updated_at),
    onSuccess: () => {
      setDeleting(null);
      toast.add({ title: t('subscriptions.ruleSets.deleted'), type: 'success' });
    },
    onError: (error) => {
      const stale = error instanceof ApiRequestError && error.status === 412;
      toast.add({ title: stale ? t('subscriptions.ruleSets.stale') : describeRequestError(error), type: stale ? 'warning' : 'error' });
      if (stale) setDeleting(null);
    },
    onSettled: () => cache.invalidateQueries({ queryKey: ['ruleSets'] }),
  });

  let editor;
  if (list.isPending) {
    editor = <SubscriptionContentState loading />;
  } else if (list.data === undefined) {
    editor = <RegionError className='flex-1' error={list.error} reload={() => void list.refetch()} title={t('subscriptions.ruleSets.loadFailed')} />;
  } else if (!ruleSet) {
    editor = <SubscriptionContentState kind='rules' title={t('subscriptions.ruleSets.empty', { client: picked })} />;
  } else {
    editor = (
      <RuleSetEditor
        key={`${ruleSet.id}-${ruleSet.updated_at}`}
        onDelete={setDeleting}
        onRename={(initial, apply) => setNaming({ kind: 'rename', initial, apply })}
        ruleSet={ruleSet}
      />
    );
  }

  const picker = (
    <Select<SubscriptionFormat>
      items={CLIENTS.map(value => ({ label: value, value }))}
      onValueChange={(value) => {
        if (value && value !== picked) {
          confirmNavigation(() => {
            setPicked(value);
            setSelected(null);
          });
        }
      }}
      value={picked}
    >
      <SelectTrigger
        aria-label={t('subscriptions.ruleSets.client')}
        className='h-10 w-full gap-2 rounded-[var(--radius-control)] border-transparent bg-transparent ps-2.5 pe-2 hover:bg-[var(--color-hover)] data-popup-open:bg-[var(--color-hover)] dark:bg-transparent'
      >
        <span className='flex-none text-xs text-[var(--color-text-secondary)]'>{t('subscriptions.ruleSets.client')}</span>
        <SelectValue className='font-[family-name:var(--font-data)] font-medium' />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          {CLIENTS.map(value => (
            <SelectItem key={value} value={value}>
              <span className='font-[family-name:var(--font-data)]'>{value}</span>
              <span className='ms-auto font-[family-name:var(--font-data)] text-xs text-[var(--color-text-faint)] tabular-nums'>{counts.get(value) ?? 0}</span>
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );

  return (
    <div className='flex h-full min-h-0 min-w-0 flex-col'>
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='flex w-full min-w-0 flex-1 items-center gap-2'>
          <StripSearch label={t('subscriptions.ruleSets.search')} onValueChange={setSearch} value={search} />
          <StripDivider />
          <Button
            aria-label={t('subscriptions.ruleSets.add')}
            className='max-sm:w-(--control-height-sm) max-sm:px-0'
            onClick={() => setNaming({ kind: 'create' })}
            size='sm'
            variant='flat'
          >
            <CirclePlus aria-hidden='true' data-icon='inline-start' />
            <span className='max-sm:sr-only'>{t('subscriptions.ruleSets.add')}</span>
          </Button>
        </div>
      </ToolbarActions>
      <div className='grid min-h-0 flex-1 grid-cols-[16rem_minmax(0,1fr)] gap-3 max-md:grid-cols-1 max-md:grid-rows-[minmax(0,13rem)_minmax(0,1fr)]'>
        <Rail label={t('subscriptions.ruleSets.list', { client: picked })} top={picker}>
          {shown.map(set => (
            <RailItem
              current={set.id === ruleSet?.id}
              end={<RailCount>{set.rules.length}</RailCount>}
              icon={<Layers3 />}
              key={set.id}
              name={set.name}
              onClick={() => set.id !== ruleSet?.id && confirmNavigation(() => setSelected(set.id))}
              sub={set.channels.length > 0 ? t('subscriptions.ruleSets.usedBy', { count: set.channels.length }) : t('subscriptions.ruleSets.unused')}
            />
          ))}
        </Rail>
        {/* Loading, empty and failed states take the editor's card, so the region keeps its frame. */}
        {ruleSet && list.data !== undefined
          ? <div className='flex min-h-0 min-w-0 flex-col *:flex-1'>{editor}</div>
          : <div className='flex min-h-0 min-w-0 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]'>{editor}</div>}
      </div>
      {naming
        ? (
            <NameDialog
              initial={naming.kind === 'rename' ? naming.initial : ''}
              onClose={() => setNaming(null)}
              onSubmit={async (name) => {
                if (naming.kind === 'rename') {
                  naming.apply(name);
                  setNaming(null);
                  return;
                }
                await create.mutateAsync(name);
              }}
              taken={sets.filter(set => naming.kind === 'create' || set.id !== ruleSet?.id).map(set => set.name)}
              title={t(naming.kind === 'create' ? 'subscriptions.ruleSets.addTitle' : 'subscriptions.ruleSets.renameTitle', { client: picked })}
            />
          )
        : null}
      <Dialog open={deleting !== null} onOpenChange={(open) => {
        if (!open && !remove.isPending) setDeleting(null);
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('subscriptions.ruleSets.deleteTitle')}</DialogTitle>
            <DialogDescription>
              {deleting
                ? deleting.channels.length > 0
                  ? t('subscriptions.ruleSets.deleteUsed', {
                      channels: deleting.channels.map(channel => channel.name).join(t('subscriptions.sources.protocolSeparator')),
                      name: deleting.name,
                    })
                  : t('subscriptions.ruleSets.deleteUnused', { name: deleting.name })
                : null}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button onClick={() => setDeleting(null)} variant='outline'>{t('common.cancel')}</Button>
            <Button aria-busy={remove.isPending || undefined} onClick={() => deleting && !remove.isPending && remove.mutate(deleting)} variant='destructive'>
              {t('subscriptions.ruleSets.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

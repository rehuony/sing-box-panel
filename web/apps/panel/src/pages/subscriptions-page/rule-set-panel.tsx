import type { FormEvent, ReactNode } from 'react';

import { CSS } from '@dnd-kit/utilities';
import { useTranslation } from 'react-i18next';
import { useLocation } from 'react-router-dom';
import { CirclePlus, Trash2 } from 'lucide-react';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';
import { useDeferredValue, useId, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@shared/components/ui/input-group';
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from '@shared/components/ui/select';
import { Dialog, DialogContent, DialogDescription, DialogFields, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { SubscriptionFormat, SubscriptionRuleSet } from '@/api/api-client';

import { queries } from '@/api/queries';
import { ApiRequestError } from '@/api/api-client';
import { messageLine } from '@/utils/message-line';
import { IconAction } from '@/components/icon-action';
import { useApiClient } from '@/api/api-client-context';
import { RegionError } from '@/components/region-state';
import { usePageVisible } from '@/hooks/use-page-visible';
import { dropSlot, SortContext } from '@/components/sortable';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { useUnsavedChangesContext } from '@/stores/unsaved-changes.store';
import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';
import { describeRequestError, toastFailure } from '@/components/error-notice';
import { Rail, RailCount, RailItem, RailItemOverlay } from '@/components/rail';
import { StripAction, StripDivider, StripSearch } from '@/components/title-strip';

import type { RuleSetDraft } from './rule-set-editor';
import type { RuleSetIconKey } from './rule-set-icons';

import { ruleLine } from './rule-text';
import { RuleSetEditor } from './rule-set-editor';
import { RuleSetIconMenu } from './rule-set-icon-menu';
import { DEFAULT_RULE_SET_ICON, ruleSetIcon } from './rule-set-icons';
import { SubscriptionContentState } from './subscription-content-state';

const CLIENTS: readonly SubscriptionFormat[] = ['sing-box', 'mihomo', 'loon'];

/** The client a link to the tab names, as in `#subscription-rules/loon`. */
function linkedClient(hash: string): SubscriptionFormat | undefined {
  return hash.startsWith('#subscription-rules/') ? CLIENTS.find(client => hash.endsWith(`/${client}`)) : undefined;
}

/**
 * Names a new rule set and picks its icon from the menu at the start of the
 * name field. The name is checked here, unique among the client's sets, so
 * the dialog closes as it is submitted and the set is added in the background.
 */
function CreateDialog({ onClose, onSubmit, taken }: {
  /** Names the client's sets use, compared ignoring case. */
  taken: readonly string[];
  onSubmit: (set: { icon: RuleSetIconKey; name: string }) => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [icon, setIcon] = useState<RuleSetIconKey>(DEFAULT_RULE_SET_ICON);
  const [name, setName] = useState('');
  const [message, setMessage] = useState('');
  const nameRef = useRef<HTMLInputElement>(null);

  function submit(event: FormEvent) {
    event.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) {
      setMessage(t('subscriptions.ruleSets.validation.setName'));
    } else if (taken.some(each => each.toLowerCase() === trimmed.toLowerCase())) {
      setMessage(t('subscriptions.ruleSets.validation.setNameTaken', { name: trimmed }));
    } else {
      onSubmit({ icon, name: trimmed });
    }
  }

  return (
    <Dialog open onOpenChange={open => !open && onClose()}>
      {/* The name is what the dialog asks for; the icon before it is optional. */}
      <DialogContent initialFocus={nameRef}>
        <form className='contents' noValidate onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{t('subscriptions.ruleSets.addTitle')}</DialogTitle>
          </DialogHeader>
          <DialogFields>
            <label htmlFor={id}>{t('subscriptions.ruleSets.field.setName')}</label>
            {/* The field deepens for the name alone; the icon button marks its own focus. */}
            <InputGroup className='has-focus-visible:border-input has-[input:focus-visible]:border-[var(--input-focus)]'>
              <InputGroupAddon className='gap-1.5 py-0 ps-1'>
                <RuleSetIconMenu onChange={setIcon} value={icon} variant='field' />
                <span aria-hidden='true' className='h-5 w-px bg-border' />
              </InputGroupAddon>
              <InputGroupInput
                aria-invalid={Boolean(message) || undefined}
                autoComplete='off'
                id={id}
                maxLength={128}
                onChange={(event) => {
                  setMessage('');
                  setName(event.target.value);
                }}
                ref={nameRef}
                value={name}
              />
            </InputGroup>
          </DialogFields>
          <DialogFooter className='flex-nowrap' message={messageLine(message)}>
            <Button onClick={onClose} type='button' variant='outline'>{t('common.cancel')}</Button>
            <Button type='submit'>{t('subscriptions.ruleSets.create')}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** What a rule set's rail item shows on its one line, as its draft stands while it is edited. */
interface RailFace {
  name: string;
  end: ReactNode;
  icon: ReactNode;
  marker?: ReactNode;
}

/** A rule set in the rail that drags within its client's order while sorting is on. */
function SortableRuleSetItem({ actions, busy, current, face, id, onClick, sortable }: {
  id: string;
  face: RailFace;
  current: boolean;
  actions?: ReactNode;
  sortable: boolean;
  /** Whether the set is still being created. */
  busy?: boolean;
  onClick: () => void;
}) {
  const { t } = useTranslation();
  const reducedMotion = usePrefersReducedMotion();
  const { attributes, isDragging, listeners, setNodeRef, transform, transition }
    = useSortable({ id, disabled: !sortable });
  return (
    <RailItem
      {...attributes}
      {...listeners}
      {...face}
      actions={actions}
      aria-busy={busy || undefined}
      aria-disabled={undefined}
      aria-keyshortcuts={sortable ? 'F2' : undefined}
      aria-roledescription={t('subscriptions.ruleSets.sortableItem')}
      className='touch-pan-y select-none'
      current={current}
      item={{
        className: isDragging ? dropSlot : undefined,
        ref: setNodeRef,
        style: { transform: CSS.Translate.toString(transform), transition: reducedMotion ? undefined : transition },
      }}
      onClick={onClick}
    />
  );
}

/** One client's order: its sets by ids, then any set the ids miss. */
interface RuleSetOrder {
  ids: readonly string[];
  format: SubscriptionFormat;
}

/** The sets with the order's client's sets in its order. */
function withOrder(sets: SubscriptionRuleSet[], { format, ids }: RuleSetOrder) {
  const own = sets.filter(set => set.client === format);
  const byID = new Map(own.map(set => [set.id, set]));
  const placed = new Set(ids);
  return [
    ...sets.filter(set => set.client !== format),
    ...ids.flatMap(id => byID.get(id) ?? []),
    ...own.filter(set => !placed.has(set.id)),
  ];
}

/**
 * The rule sets tab: a client picker above the rail of that client's rule
 * sets in their saved order, and the selected set's editor beside it. The
 * rail shows the set being edited as its draft stands, deletes sets and,
 * outside a search, drags them into a new order that saves at once; the strip
 * searches set names and rules and adds a set to the picked client.
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
  const location = useLocation();
  const [picked, setPicked] = useState<SubscriptionFormat>(() => linkedClient(location.hash) ?? 'mihomo');
  const [selected, setSelected] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const query = useDeferredValue(search.trim().toLowerCase());
  const [naming, setNaming] = useState(false);
  const [deleting, setDeleting] = useState<SubscriptionRuleSet | null>(null);
  const [draft, setDraft] = useState<RuleSetDraft | null>(null);
  // A set just created opens with its prompt line focused, once.
  const [created, setCreated] = useState<string | null>(null);
  // Sets submitted and not yet created, shown at the end of their client's rail, and the IDs they were given.
  const [adding, setAdding] = useState<SubscriptionRuleSet[]>([]);
  const addedRef = useRef(new Map<string, string>());
  // The order the last drop is saving, shown from the drop until the save settles.
  const [order, setOrder] = useState<RuleSetOrder | null>(null);
  // A drag completes a click on the item it started from; that click is not the user's.
  const draggingRef = useRef(false);
  const suppressClickRef = useRef(false);

  const all = useMemo(() => list.data ?? [], [list.data]);
  const counts = useMemo(
    () => new Map(CLIENTS.map(each => [each, all.filter(set => set.client === each).length])),
    [all],
  );
  const sets = useMemo(() => {
    const ordered = order?.format === picked ? withOrder(all, order) : all;
    return [...ordered, ...adding].filter(set => set.client === picked);
  }, [adding, all, order, picked]);
  const isAdding = (id: string) => adding.some(set => set.id === id);
  const rulesOf = (set: SubscriptionRuleSet) => draft?.id === set.id ? draft.rules : set.rules;
  const hits = (set: SubscriptionRuleSet) =>
    rulesOf(set).filter(rule => ruleLine(rule).toLowerCase().includes(query)).length;
  // The set open in the editor, picked or first shown, stays open and in the rail whatever the search finds.
  const chosen = sets.find(set => set.id === selected) ?? sets.find(set => set.id === draft?.id);
  const shown = sets.filter(set => query === '' || set.id === chosen?.id || set.name.toLowerCase().includes(query) || hits(set) > 0);
  const ruleSet = chosen ?? shown[0] ?? null;

  // The response is the whole new set, so the list is not read again.
  const create = useMutation({
    mutationFn: (set: SubscriptionRuleSet) =>
      client.createSubscriptionRuleSet({ client: set.client, icon: set.icon, name: set.name, rules: [] }),
    onSuccess: (saved, set) => {
      addedRef.current.set(set.id, saved.id);
      cache.setQueryData<SubscriptionRuleSet[]>(['ruleSets'], current => current && [...current, saved]);
      setAdding(current => current.filter(each => each.id !== set.id));
      setCreated(saved.id);
      setSelected(current => current === set.id ? saved.id : current);
    },
    onError: (error, set) => {
      setAdding(current => current.filter(each => each.id !== set.id));
      setSelected(current => current === set.id ? null : current);
      toast.add({ title: t('subscriptions.ruleSets.createFailed', { name: set.name }), description: describeRequestError(error), type: 'error' });
    },
  });

  function add({ icon, name }: { icon: RuleSetIconKey; name: string }) {
    const set: SubscriptionRuleSet = {
      id: `adding:${crypto.randomUUID()}`, client: picked, icon, name, rules: [], channels: [], created_at: '', updated_at: '',
    };
    setAdding(current => [...current, set]);
    setNaming(false);
    // Leaving an unsaved draft may be confirmed after the set has its ID.
    confirmNavigation(() => setSelected(addedRef.current.get(set.id) ?? set.id));
    create.mutate(set);
  }
  const remove = useMutation({
    mutationFn: (set: SubscriptionRuleSet) => client.deleteSubscriptionRuleSet(set.id, set.updated_at),
    onSuccess: () => {
      setDeleting(null);
      toast.add({ title: t('subscriptions.ruleSets.deleted'), type: 'success' });
    },
    onError: (error) => {
      const stale = error instanceof ApiRequestError && error.status === 412;
      if (stale) toast.add({ title: t('subscriptions.ruleSets.stale'), type: 'warning' });
      else toastFailure(t('subscriptions.ruleSets.deleteFailed'), error);
      if (stale) setDeleting(null);
    },
    onSettled: () => cache.invalidateQueries({ queryKey: ['ruleSets'] }),
  });
  // Drops save one after another, each the whole order, so the last one wins; a list read meanwhile
  // gives way to the saved order, and an order that does not save goes back to the list read again.
  const saveOrder = useMutation({
    scope: { id: 'rule-set-order' },
    mutationFn: (next: RuleSetOrder) => client.setSubscriptionRuleSetOrder(next.format, [...next.ids]),
    onSuccess: async (_saved, next) => {
      await cache.cancelQueries({ queryKey: ['ruleSets'] });
      cache.setQueryData<SubscriptionRuleSet[]>(['ruleSets'], current => current && withOrder(current, next));
    },
    onError: (error) => {
      const stale = error instanceof ApiRequestError && error.status === 409;
      if (stale) toast.add({ title: t('subscriptions.ruleSets.orderStale'), type: 'warning' });
      else toastFailure(t('subscriptions.ruleSets.orderSaveFailed'), error);
      void cache.invalidateQueries({ queryKey: ['ruleSets'] });
    },
    onSettled: (_saved, _error, next) => setOrder(current => current === next ? null : current),
  });
  // Sets reorder only as a whole list: not while a search hides some, nor while a new one has no ID yet.
  const sortable = query === '' && !sets.some(set => isAdding(set.id));

  function move(active: string, over: string) {
    const ids = sets.map(set => set.id);
    const from = ids.indexOf(active);
    const to = ids.indexOf(over);
    if (!sortable || from < 0 || to < 0 || from === to) return;
    // The new order shows in the same render as the drop, so nothing jumps back while it saves.
    const next = { format: picked, ids: arrayMove(ids, from, to) };
    setOrder(next);
    saveOrder.mutate(next);
  }

  function faceOf(set: SubscriptionRuleSet): RailFace {
    const editing = draft?.id === set.id ? draft : null;
    const icon = ruleSetIcon(editing?.icon ?? set.icon);
    return {
      // While searching, a set counts the rules that match.
      end: <RailCount>{query === '' ? rulesOf(set).length : hits(set)}</RailCount>,
      icon: <icon.Icon />,
      marker: editing?.dirty
        ? <span aria-label={t('subscriptions.ruleSets.unsavedMark')} className='size-1.5 flex-none rounded-full bg-primary' role='img' />
        : undefined,
      name: editing?.name ?? set.name,
    };
  }

  let editor;
  if (list.isPending) {
    editor = <SubscriptionContentState loading />;
  } else if (list.data === undefined) {
    editor = <RegionError className='flex-1' error={list.error} reload={() => void list.refetch()} title={t('subscriptions.ruleSets.loadFailed')} />;
  } else if (!ruleSet) {
    editor = <SubscriptionContentState kind='rules' title={t('subscriptions.ruleSets.empty', { client: picked })} />;
  } else if (isAdding(ruleSet.id)) {
    editor = <SubscriptionContentState loading />;
  } else {
    editor = (
      <RuleSetEditor
        focusEntry={created === ruleSet.id}
        key={`${ruleSet.id}-${ruleSet.updated_at}`}
        onDraftChange={setDraft}
        onEntryFocused={() => setCreated(null)}
        query={query}
        ruleSet={ruleSet}
        takenNames={sets.flatMap(set => set.id === ruleSet.id ? [] : [set.name])}
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
          <StripAction
            icon={<CirclePlus aria-hidden='true' data-icon='inline-start' />}
            label={t('subscriptions.ruleSets.add')}
            onClick={() => setNaming(true)}
          />
        </div>
      </ToolbarActions>
      <div className='grid min-h-0 flex-1 grid-cols-[16rem_minmax(0,1fr)] gap-3 max-md:grid-cols-1 max-md:grid-rows-[minmax(0,13rem)_minmax(0,1fr)]'>
        <div
          className='contents'
          onClickCapture={(event) => {
            if (!suppressClickRef.current) return;
            event.preventDefault();
            event.stopPropagation();
          }}
          onKeyDownCapture={(event) => {
            if (!draggingRef.current && event.key !== 'F2') suppressClickRef.current = false;
          }}
          onPointerDownCapture={() => {
            if (!draggingRef.current) suppressClickRef.current = false;
          }}
        >
          <Rail label={t('subscriptions.ruleSets.list', { client: picked })} top={picker}>
            <SortContext
              ended={t('subscriptions.ruleSets.dragEnd')}
              instructions={t('subscriptions.ruleSets.dragInstructions')}
              items={shown.map(set => ({ id: set.id, label: faceOf(set).name }))}
              onDraggingChange={(dragging) => {
                draggingRef.current = dragging;
                if (dragging) suppressClickRef.current = true;
              }}
              onMove={move}
              renderOverlay={(id) => {
                const set = sets.find(each => each.id === id);
                return set ? <RailItemOverlay {...faceOf(set)} wideEnd /> : null;
              }}
              strategy={verticalListSortingStrategy}
            >
              {shown.map(set => (
                <SortableRuleSetItem
                  actions={isAdding(set.id)
                    ? undefined
                    : (
                        <IconAction label={t('subscriptions.ruleSets.delete')} onClick={() => setDeleting(set)} size='icon-xs'>
                          <Trash2 aria-hidden='true' />
                        </IconAction>
                      )}
                  busy={isAdding(set.id)}
                  current={set.id === ruleSet?.id}
                  face={faceOf(set)}
                  id={set.id}
                  key={set.id}
                  onClick={() => set.id !== ruleSet?.id && confirmNavigation(() => setSelected(set.id))}
                  sortable={sortable}
                />
              ))}
            </SortContext>
          </Rail>
        </div>
        {/* Loading, empty and failed states take the editor's card, so the region keeps its frame. */}
        {ruleSet && list.data !== undefined && !isAdding(ruleSet.id)
          ? <div className='flex min-h-0 min-w-0 flex-col *:flex-1'>{editor}</div>
          : <div className='flex min-h-0 min-w-0 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]'>{editor}</div>}
      </div>
      {naming
        ? (
            <CreateDialog
              onClose={() => setNaming(false)}
              onSubmit={add}
              taken={sets.map(set => set.name)}
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

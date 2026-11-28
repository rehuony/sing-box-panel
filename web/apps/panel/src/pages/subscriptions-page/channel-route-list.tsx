import type { SyntheticEvent } from 'react';
import type { DraggableSyntheticListeners, KeyboardCodes } from '@dnd-kit/core';

import { cn } from '@shared/lib/utils';
import { CSS } from '@dnd-kit/utilities';
import { createPortal } from 'react-dom';
import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useState } from 'react';
import { Button } from '@shared/components/ui/button';
import { EM_DASH } from '@shared/lib/telemetry-format';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { closestCenter, DndContext, DragOverlay } from '@dnd-kit/core';
import { CirclePlus, GripVertical, Link2, ListFilter, PencilLine, Trash2 } from 'lucide-react';
import { arrayMove, SortableContext, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from '@shared/components/ui/dropdown-menu';

import type {
  ChannelRoute,
  ChannelRouteExit,
  SubscriptionChannelConfig,
  SubscriptionFormat,
  SubscriptionNode,
  SubscriptionRule,
  SubscriptionRuleSet,
} from '@/api/api-client';

import { IconAction } from '@/components/icon-action';
import { InfoTooltip } from '@/components/info-tooltip';
import { SelectField } from '@/components/select-field';
import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';
import { dropSlot, showGrabbing, useSortSensors, useSortWindow } from '@/components/sortable';

import { ruleType } from './rule-text';
import { RuleEditor } from './rule-editor';
import { exitValue, parseExit, useExits } from './channel-exits';
import { ChannelRuleSetPicker } from './channel-rule-set-picker';
import { SubscriptionContentState } from './subscription-content-state';
import { exitsThrough, loonAheadOf, routeRuleSet, unusableRemote } from './channel-policy';

/**
 * A row's cells: grip, position, what it matches, its actions and its exit,
 * which ends the row at its edge; the exit and actions take a second line on
 * a phone, the actions after the exit.
 */
// The actions column holds two embedded actions, which grow to 44px on a coarse pointer.
const row = 'grid min-h-13 grid-cols-[1.75rem_2rem_minmax(0,1fr)_calc(2*var(--control-height-xs)+2px)_9rem] items-center gap-x-2 gap-y-1.5 py-1.5 ps-2 pe-3 max-sm:grid-cols-[1.75rem_1.5rem_minmax(0,1fr)_calc(2*var(--control-height-xs)+2px)]';
const exitCell = 'h-(--control-height-sm) w-36 text-compact sm:col-start-5 sm:row-start-1 max-sm:col-start-3 max-sm:row-start-2 max-sm:w-full';
const actionsCell = 'flex justify-end gap-0.5 sm:col-start-4 sm:row-start-1 max-sm:col-start-4 max-sm:row-start-2';
const secondary = 'text-xs text-[var(--color-text-secondary)]';
const danger = 'text-xs text-[var(--color-danger-text)]';
const positionCell = 'text-end font-[family-name:var(--font-data)] text-xs text-[var(--color-text-faint)] tabular-nums sm:pe-2';
/** A row's grip picks it up and drops it with F2, Space or Enter; Escape cancels. */
const routeKeys: KeyboardCodes = { start: ['F2', 'Space', 'Enter'], end: ['F2', 'Space', 'Enter'], cancel: ['Escape'] };

/**
 * A row's pointer listeners, which start a drag from anywhere on the row but
 * its controls; the grip alone takes the keyboard, so Space and Enter on the
 * exit or an action keep their own meaning. Events from a control's popup,
 * which React bubbles here from a portal, start nothing.
 */
function rowPointerListeners(listeners: DraggableSyntheticListeners): NonNullable<DraggableSyntheticListeners> {
  const pointer = Object.entries(listeners ?? {}).filter(([name]) => name !== 'onKeyDown');
  return Object.fromEntries(pointer.map(([name, handler]) => [name, (event: SyntheticEvent<HTMLElement>) => {
    const target = event.target as Element;
    const control = target.closest('button, [role=combobox]');
    if (!event.currentTarget.contains(target) || (control && !control.hasAttribute('data-route-grip'))) return;
    handler(event);
  }]));
}

/** Remote rule set names the channel's own rules use, besides the one being edited. */
function ownRemoteNames(routes: readonly ChannelRoute[], except?: string): string[] {
  return routes.flatMap(route => route.id !== except && route.rule?.remote ? [route.rule.remote.name] : []);
}

/**
 * Adds exit rules: the client's rule sets, each once a channel, or a rule of
 * the channel's own. New rules are enabled and take the given exit.
 */
export function ChannelRouteAdd({ busy, exit, exitName, format, onAdd, routes, ruleSets }: {
  busy: boolean;
  /** The exit new rules take, and its name. */
  exit: ChannelRouteExit;
  exitName: string;
  format: SubscriptionFormat;
  routes: readonly ChannelRoute[];
  ruleSets: readonly SubscriptionRuleSet[] | undefined;
  onAdd: (routes: ChannelRoute[]) => void;
}) {
  const { t } = useTranslation();
  const [adding, setAdding] = useState<{ remote: boolean } | null>(null);
  const used = new Set(routes.flatMap(route => route.rule_set_id === undefined ? [] : [route.rule_set_id]));
  function route(match: { rule_set_id: string } | { rule: SubscriptionRule }): ChannelRoute {
    return { id: crypto.randomUUID(), enabled: true, ...match, exit };
  }
  return (
    <>
      <ChannelRuleSetPicker
        busy={busy}
        exitName={exitName}
        format={format}
        onAdd={ids => onAdd(ids.map(id => route({ rule_set_id: id })))}
        ruleSets={ruleSets}
        used={used}
      />
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button aria-label={t('channels.addCustomRule')} className='max-sm:w-(--control-height-sm) max-sm:px-0' disabled={busy} size='sm' variant='flat' />}>
          <CirclePlus aria-hidden='true' data-icon='inline-start' />
          <span className='max-sm:sr-only'>{t('channels.addCustomRule')}</span>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' className='w-auto min-w-40'>
          <DropdownMenuGroup>
            <DropdownMenuItem onClick={() => setAdding({ remote: false })}>
              <ListFilter aria-hidden='true' />
              {t('channels.matchRule')}
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setAdding({ remote: true })}>
              <Link2 aria-hidden='true' />
              {t('channels.remoteRule')}
            </DropdownMenuItem>
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>
      {adding
        ? (
            <RuleEditor
              client={format}
              onClose={() => setAdding(null)}
              onSave={(rule) => {
                onAdd([route({ rule })]);
                setAdding(null);
              }}
              remote={adding.remote}
              rule={null}
              takenNames={ownRemoteNames(routes)}
            />
          )
        : null}
    </>
  );
}

/**
 * What a route matches: a rule set's name over its rule count, or an own
 * rule's value over its kind. A rule the client matches ahead of earlier ones
 * ends its name in a warning sign that names them.
 */
function RouteMatch({ aheadOf, format, route, ruleSets }: {
  route: ChannelRoute;
  format: SubscriptionFormat;
  ruleSets: readonly SubscriptionRuleSet[] | undefined;
  /** The positions of the earlier rules the client matches this one ahead of. */
  aheadOf?: readonly number[];
}) {
  const { t } = useTranslation();
  const off = route.enabled ? '' : ` · ${t('channels.disabled')}`;
  const ahead = aheadOf?.length
    ? t('channels.loonAheadOf', { count: aheadOf.length, positions: aheadOf.join(t('channels.positionSeparator')) })
    : '';
  const mark = ahead ? <InfoTooltip className='-my-1 flex-none' label={ahead} tone='warning'>{ahead}</InfoTooltip> : null;
  if (route.rule_set_id !== undefined) {
    const set = routeRuleSet(route, ruleSets, format);
    const known = ruleSets?.find(value => value.id === route.rule_set_id);
    let detail = <p className={secondary}>{t('channels.ruleSet')}</p>;
    if (set) detail = <p className={secondary}>{t('channels.routeRuleSet', { count: set.rules.length }) + off}</p>;
    else if (ruleSets) detail = <p className={danger}>{t('channels.routeRuleSetMissing', { client: format })}</p>;
    return (
      <div className='min-w-0 max-sm:col-span-2'>
        <p className='flex min-w-0 items-center gap-1 text-sm'>
          <span className='min-w-0 break-words'>{known?.name ?? (ruleSets ? route.rule_set_id : EM_DASH)}</span>
          {mark}
        </p>
        {detail}
      </div>
    );
  }
  const rule = route.rule;
  const kind = ruleType(rule?.kind ?? 'domain');
  const unusable = unusableRemote(rule, format);
  return (
    <div className='min-w-0 max-sm:col-span-2' title={rule?.remote?.url}>
      <p className='flex min-w-0 items-center gap-1'>
        <span className='min-w-0 font-[family-name:var(--font-data)] text-compact break-all'>{(rule?.kind === 'remote' ? rule.remote?.name : rule?.value) ?? EM_DASH}</span>
        {mark}
      </p>
      <p className={unusable ? danger : secondary}>
        {t('channels.routeCustom', { kind })}
        {unusable ? ` · ${t('channels.routeFormatUnusable', { client: format, format: rule!.remote!.format })}` : off}
      </p>
    </div>
  );
}

function RouteRow({ aheadOf, busy, choices, format, index, onEdit, onExit, onRemove, route, ruleSets, sortable }: {
  route: ChannelRoute;
  aheadOf?: readonly number[];
  /** Its position among all the channel's routes. */
  index: number;
  sortable: boolean;
  busy: boolean;
  format: SubscriptionFormat;
  ruleSets: readonly SubscriptionRuleSet[] | undefined;
  choices: { value: string; label: string }[];
  onExit: (exit: ChannelRouteExit) => void;
  onEdit: () => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const reducedMotion = usePrefersReducedMotion();
  // While the channel saves the list's sensors start no drag, so saving re-registers no row.
  const { attributes, isDragging, listeners, setActivatorNodeRef, setNodeRef, transform, transition }
    = useSortable({ id: route.id, disabled: !sortable });
  const position = index + 1;
  const draggable = sortable && !busy;
  return (
    <li
      {...(draggable ? rowPointerListeners(listeners) : {})}
      className={cn(
        row,
        'group/route border-b transition-colors duration-[var(--dur-micro)] ease-out',
        draggable && 'cursor-grab touch-pan-y select-none hover:bg-[var(--color-hover)]',
        isDragging && dropSlot,
        !route.enabled && 'text-muted-foreground',
      )}
      data-dragging={isDragging || undefined}
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform), transition: reducedMotion ? undefined : transition }}
    >
      <span className='flex justify-center'>
        {sortable
          ? (
              <button
                {...attributes}
                aria-label={t('channels.moveRoute', { position })}
                className='flex size-7 cursor-grab touch-none items-center justify-center rounded-[var(--radius-control)] text-[var(--color-text-faint)] outline-none group-hover/route:text-[var(--color-text-secondary)] hover:text-foreground focus-visible:shadow-[inset_0_0_0_1px_var(--ring)] disabled:cursor-default'
                data-route-grip=''
                disabled={busy}
                onKeyDown={listeners?.onKeyDown as ((event: SyntheticEvent) => void) | undefined}
                ref={setActivatorNodeRef}
                type='button'
              >
                <GripVertical aria-hidden='true' className='size-4' />
              </button>
            )
          : null}
      </span>
      <span className={positionCell}>{position}</span>
      <RouteMatch aheadOf={aheadOf} format={format} route={route} ruleSets={ruleSets} />
      <SelectField
        aria-label={t('channels.routeExit', { position })}
        className={exitCell}
        disabled={busy}
        items={choices}
        onValueChange={value => onExit(parseExit(value))}
        value={exitValue(route.exit)}
      />
      <span className={cn(actionsCell, 'invisible group-focus-within/route:visible group-hover/route:visible [@media(hover:none)]:visible')}>
        {route.rule
          ? (
              <IconAction disabled={busy} label={t('common.edit')} onClick={onEdit} size='icon-xs'>
                <PencilLine aria-hidden='true' />
              </IconAction>
            )
          : null}
        <IconAction disabled={busy} label={t('common.remove')} onClick={onRemove} size='icon-xs'>
          <Trash2 aria-hidden='true' />
        </IconAction>
      </span>
    </li>
  );
}

/** The copy of a row the pointer carries: lifted with the panel shadow, inert. */
function RouteRowOverlay({ aheadOf, choices, format, index, route, ruleSets }: {
  route: ChannelRoute;
  aheadOf?: readonly number[];
  index: number;
  format: SubscriptionFormat;
  ruleSets: readonly SubscriptionRuleSet[] | undefined;
  choices: { value: string; label: string }[];
}) {
  return (
    <div
      aria-hidden='true'
      className={cn(row, 'cursor-grabbing rounded-[var(--radius-control)] border bg-popover shadow-[var(--shadow-panel)]', !route.enabled && 'text-muted-foreground')}
      inert
    >
      <span className='flex justify-center'>
        <span className='flex size-7 items-center justify-center text-foreground'><GripVertical className='size-4' /></span>
      </span>
      <span className={positionCell}>{index + 1}</span>
      <RouteMatch aheadOf={aheadOf} format={format} route={route} ruleSets={ruleSets} />
      <SelectField className={exitCell} items={choices} onValueChange={() => undefined} value={exitValue(route.exit)} />
      <span className={cn(actionsCell, 'max-sm:hidden')} />
    </div>
  );
}

/**
 * A channel's exit rules as one block of rows in matching order, sorted by
 * dragging; a long list draws the rows near its view and the one dragged.
 * Filtered to the rules that leave through a group, the rows keep their
 * positions in the whole list but do not sort. The final exit, which takes
 * what no rule matches, is set on a group in the rail.
 */
export function ChannelRouteList({
  busy, config, format, groupID, nodes, onRoutesChange, ruleSets,
}: {
  busy: boolean;
  config: SubscriptionChannelConfig;
  format: SubscriptionFormat;
  /** The pool, which names node exits. */
  nodes: readonly SubscriptionNode[];
  ruleSets: readonly SubscriptionRuleSet[] | undefined;
  /** Shows only the rules that leave through this group. */
  groupID?: string;
  onRoutesChange: (routes: ChannelRoute[]) => void;
}) {
  const { t } = useTranslation();
  const { routes } = config;
  const exits = useExits(config.groups, nodes);
  const [editing, setEditing] = useState<ChannelRoute | null>(null);
  const [dragged, setDragged] = useState<string | null>(null);
  // A drag cut short by leaving the view must not leave the hand closed.
  useEffect(() => () => showGrabbing(false), []);
  const sensors = useSortSensors(busy, routeKeys);
  const shown = useMemo(() => routes.flatMap((route, index) =>
    groupID === undefined || exitsThrough(route.exit, groupID) ? [{ index, route }] : []), [groupID, routes]);
  const ids = useMemo(() => shown.map(({ route }) => route.id), [shown]);
  const { ref, slots, style } = useSortWindow(ids, dragged);
  const position = (id: string | number) => routes.findIndex(route => route.id === id) + 1;
  const draggedRow = dragged === null ? undefined : shown.find(({ route }) => route.id === dragged);
  const ahead = useMemo(() => loonAheadOf(routes, ruleSets, format), [format, routes, ruleSets]);
  const replace = (id: string, next: Partial<ChannelRoute>) =>
    onRoutesChange(routes.map(route => route.id === id ? { ...route, ...next } : route));

  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      {shown.length
        ? (
            <ScrollArea className='min-h-0 flex-1'>
              <DndContext
                accessibility={{
                  // The live region sits in the body, so a dialog around the list still hides behind a nested one,
                  // and no announcement lands inside a table's markup.
                  container: document.body,
                  screenReaderInstructions: { draggable: t('channels.routeDragInstructions') },
                  announcements: {
                    onDragStart: ({ active }) => t('channels.routeDragStart', { position: position(active.id) }),
                    onDragOver: ({ over }) => over ? t('common.dragPosition', { position: position(over.id), count: routes.length }) : undefined,
                    onDragEnd: () => t('channels.routeDragEnd'),
                    onDragCancel: () => t('common.dragCancel'),
                  },
                }}
                collisionDetection={closestCenter}
                onDragCancel={() => {
                  setDragged(null);
                  showGrabbing(false);
                }}
                onDragEnd={({ active, over }) => {
                  setDragged(null);
                  showGrabbing(false);
                  const from = position(active.id) - 1;
                  const to = over ? position(over.id) - 1 : -1;
                  if (from >= 0 && to >= 0 && from !== to) onRoutesChange(arrayMove(routes, from, to));
                }}
                onDragStart={({ active }) => {
                  setDragged(String(active.id));
                  showGrabbing(true);
                }}
                sensors={sensors}
              >
                <SortableContext items={ids} strategy={verticalListSortingStrategy}>
                  <ol aria-label={t('channels.routes')} ref={ref} style={style}>
                    {slots.map((slot) => {
                      if (typeof slot !== 'number') return <li key='gap' {...slot} />;
                      const { index, route } = shown[slot]!;
                      return (
                        <RouteRow
                          aheadOf={ahead.get(route.id)}
                          busy={busy}
                          choices={exits.choices(route.exit)}
                          format={format}
                          index={index}
                          key={route.id}
                          onEdit={() => setEditing(route)}
                          onExit={exit => replace(route.id, { exit })}
                          onRemove={() => onRoutesChange(routes.filter(value => value.id !== route.id))}
                          route={route}
                          ruleSets={ruleSets}
                          sortable={groupID === undefined}
                        />
                      );
                    })}
                  </ol>
                </SortableContext>
                {createPortal(
                  <DragOverlay dropAnimation={null} style={{ zIndex: 'calc(var(--z-modal) + 2)' }}>
                    {draggedRow
                      ? (
                          <RouteRowOverlay
                            aheadOf={ahead.get(draggedRow.route.id)}
                            choices={exits.choices(draggedRow.route.exit)}
                            format={format}
                            index={draggedRow.index}
                            route={draggedRow.route}
                            ruleSets={ruleSets}
                          />
                        )
                      : null}
                  </DragOverlay>,
                  document.body,
                )}
              </DndContext>
            </ScrollArea>
          )
        : <SubscriptionContentState kind='rules' title={t(groupID === undefined ? 'channels.noRoutes' : 'channels.noGroupRoutes')} />}
      {editing?.rule
        ? (
            <RuleEditor
              client={format}
              onClose={() => setEditing(null)}
              onSave={(rule) => {
                replace(editing.id, { rule });
                setEditing(null);
              }}
              remote={editing.rule.kind === 'remote'}
              rule={editing.rule}
              takenNames={ownRemoteNames(routes, editing.id)}
            />
          )
        : null}
    </div>
  );
}

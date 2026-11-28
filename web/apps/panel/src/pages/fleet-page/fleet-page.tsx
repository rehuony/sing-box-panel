import type { MouseEvent } from 'react';

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { useShallow } from 'zustand/react/shallow';
import { ListChecks, SearchX, Server } from 'lucide-react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { Tabs, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { useCallback, useDeferredValue, useEffect, useMemo } from 'react';

import type { NodeStatus } from '@/utils/fleet-summary';
import type { FleetStateFilter } from '@/stores/fleet-view.store';

import { queries } from '@/api/queries';
import { fleetTags } from '@/utils/panel-commands';
import { fleetFigures } from '@/utils/fleet-summary';
import { useApiClient } from '@/api/api-client-context';
import { SelectField } from '@/components/select-field';
import { useFleetView } from '@/stores/fleet-view.store';
import { LoadingState } from '@/components/loading-state';
import { usePageVisible } from '@/hooks/use-page-visible';
import { usePhoneLayout } from '@/hooks/use-phone-layout';
import { AddNodeAction } from '@/components/enrollment-dialog';
import { useCommandSession } from '@/stores/command-session.store';
import { RegionError, RegionState } from '@/components/region-state';
import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';
import { StripAction, StripDivider, StripSearch, TitleStrip } from '@/components/title-strip';

import { NodePane } from './node-pane';
import { FleetFigures } from './fleet-figures';

const FILTERS = ['all', 'online', 'offline'] as const satisfies readonly FleetStateFilter[];
/** Panes that always paint: more than a large window shows at once. */
const EAGER_PANES = 48;
/** The tag filter's value for every tag. */
const ALL_TAGS = '';

/** The card the panes sit on, 16px in and 12px apart, which also holds the region's loading, error and empty states. */
const paneCard = 'flex flex-col rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]';
const regionState = 'min-h-0 flex-1';

function counts(items: readonly NodeStatus[]): Record<FleetStateFilter, number> {
  const result = { all: items.length, offline: 0, online: 0 };
  for (const { node } of items) result[node.state] += 1;
  return result;
}

/**
 * Nodes that need attention lead: offline, then over quota; the rest follow
 * by name. Live load never reorders the panes, so they stay where they are
 * between refreshes.
 */
function attention({ metrics, node }: NodeStatus): number {
  if (node.state === 'offline') return 0;
  if (metrics.quota_exceeded) return 1;
  return 2;
}

function matches({ node }: NodeStatus, query: string): boolean {
  return [node.name, node.hostname, node.public_ip ?? '', ...node.tags].some(value => value.toLowerCase().includes(query));
}

/**
 * Escape leaves selection mode and clears the selection unless a field, a
 * menu, a dialog or the command line has it.
 */
function useEscapeClearsSelection(active: boolean) {
  useEffect(() => {
    if (!active) return;
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== 'Escape' || event.defaultPrevented || useCommandSession.getState().open) return;
      if (event.target instanceof Element && event.target.closest('input:not([type="checkbox"]), textarea, select, [role="dialog"], [role="alertdialog"], [role="menu"], [role="listbox"]')) return;
      useFleetView.getState().clearSelection();
    }
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [active]);
}

/** What a click on the home screen acts on rather than leaving selection mode: a pane or a control. */
const CLICK_TARGETS = 'article, a, button, input, textarea, select, label, [role="button"], [role="tab"], [role="checkbox"], [role="combobox"], [data-slot^="scroll-area-scrollbar"]';

/**
 * A click on the home screen's blank space, outside the panes and controls,
 * leaves selection mode and clears the selection. Clicks inside a layer the
 * screen opened, such as a dialog, and the click that ends a text selection
 * are not blank space.
 */
function leaveSelectionOnBlankClick(event: MouseEvent<HTMLElement>) {
  const { target } = event;
  if (!(target instanceof Element) || !event.currentTarget.contains(target) || target.closest(CLICK_TARGETS)) return;
  if (window.getSelection()?.isCollapsed === false) return;
  const { clearSelection, selected, selecting } = useFleetView.getState();
  if (selecting || selected.size > 0) clearSelection();
}

/**
 * The home screen: a title strip that filters the fleet by state and tag and
 * searches it, the fleet's key figures, then every node as a pane in a grid
 * on one card that scrolls inside it. The strip stays whole while the fleet
 * loads, fails to load or has no nodes; only what the card holds changes, and
 * with no nodes it leads with adding one whatever the filters say.
 */
export function FleetPage() {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const visible = usePageVisible();
  const reducedMotion = usePrefersReducedMotion();
  const phone = usePhoneLayout();
  const status = useQuery({ ...queries.fleetStatus(client), enabled: visible });
  const update = useQuery(queries.systemUpdate(client));
  const fleetView = useFleetView(useShallow(store => ({
    search: store.search,
    selected: store.selected,
    selecting: store.selecting,
    setFilter: store.setFilter,
    setSelecting: store.setSelecting,
    setVisible: store.setVisible,
    state: store.state,
    tag: store.tag,
    toggle: store.toggle,
  })));
  const { search, selected, selecting, setFilter, setSelecting, setVisible, state, tag } = fleetView;
  const { toggle } = fleetView;
  const query = useDeferredValue(search.trim().toLowerCase());
  const items = useMemo(() => {
    const names = new Intl.Collator(locale, { numeric: true });
    return [...status.data?.items ?? []]
      .sort((a, b) => attention(a) - attention(b) || names.compare(a.node.name, b.node.name));
  }, [locale, status.data]);
  const totals = useMemo(() => counts(items), [items]);
  const figures = useMemo(() => fleetFigures(items), [items]);
  const tags = useMemo(() => fleetTags(items), [items]);
  // A tag filter no node carries, such as one the command line set, stays a choice so the strip shows it.
  const tagChoices = tag === null || tags.includes(tag) ? tags : [...tags, tag];
  const matching = useMemo(() => items.filter(item => (state === 'all' || item.node.state === state)
    && (tag === null || item.node.tags.includes(tag))
    && (query === '' || matches(item, query))), [items, query, state, tag]);
  const order = useMemo(() => matching.map(item => item.node.id), [matching]);
  useEffect(() => setVisible(order), [order, setVisible]);
  useEscapeClearsSelection(selecting || selected.size > 0);
  const onToggle = useCallback((id: string, range: boolean) => {
    toggle(id, range ? useFleetView.getState().visible : undefined);
  }, [toggle]);
  const refresh = () => void status.refetch();
  const release = update.data?.current_version ?? null;
  // Until the fleet loads, its counts are unknown rather than zero.
  const loaded = status.data !== undefined;
  const fleet = items.length > 0;
  const figuresCard = fleet ? <FleetFigures figures={figures} /> : null;
  // On a phone the figures scroll with the panes, so the strip alone stays at the top.
  const figuresScroll = phone && matching.length > 0;

  let content;
  if (status.isPending) {
    content = <LoadingState label={t('fleet.loading')} fullScreen={false} />;
  } else if (status.data === undefined) {
    content = <RegionError className={regionState} error={status.error} reload={refresh} title={t('nodes.loadFailed')} />;
  } else if (!fleet) {
    content = <RegionState className={regionState} icon={Server} title={t('fleet.empty.title')} />;
  } else if (matching.length === 0) {
    content = <RegionState className={regionState} icon={SearchX} title={t('fleet.noMatch.title')} />;
  } else {
    const grid = (
      <ul aria-label={t('nav.nodes')} className='grid grid-cols-[repeat(auto-fill,minmax(min(100%,var(--pane-min-width)),1fr))] content-start gap-3'>
        {matching.map((item, index) => (
          // Panes past the first screenful skip layout and paint until they near the view, keeping their height;
          // the first screenful always paints with the filter change that brought it.
          <li
            className={cn('min-w-0', index >= EAGER_PANES && '[contain-intrinsic-size:auto_var(--pane-height)] [content-visibility:auto]')}
            key={item.node.id}
          >
            <NodePane
              item={item}
              morph={!reducedMotion}
              onToggle={onToggle}
              release={release}
              selected={selected.has(item.node.id)}
              selecting={selecting}
            />
          </li>
        ))}
      </ul>
    );
    // On a phone the figures and the card scroll together; elsewhere the card stays and the panes scroll inside it.
    content = figuresScroll
      ? (
          <ScrollArea className='min-h-0 flex-1' viewportClassName='flex flex-col gap-3'>
            {figuresCard}
            <div className={cn(paneCard, 'grow p-4')}>{grid}</div>
          </ScrollArea>
        )
      : <ScrollArea className='min-h-0 flex-1' viewportClassName='p-4'>{grid}</ScrollArea>;
  }

  // Size containment keeps the panes out of the workspace's height, so the
  // screen fills it exactly and the grid scrolls below the strip and figures.
  return (
    // Pointer users leave selection mode by clicking blank space; keyboards use Escape or the strip's toggle.
    <section className='flex min-h-0 min-w-0 flex-1 flex-col gap-[var(--workspace-content-gap)] [contain:size]' onClick={leaveSelectionOnBlankClick}>
      <h1 className='sr-only'>{t('nav.nodes')}</h1>
      <TitleStrip>
        {/* Filters at the start, then search filling the room between the dividers, then the actions. */}
        <Tabs className='max-w-full min-w-0' onValueChange={value => setFilter({ state: value as FleetStateFilter })} value={state}>
          <TabsList aria-label={t('fleet.filter.label')} className='max-w-full overflow-x-auto [scrollbar-width:none]' variant='pill'>
            {FILTERS.map(value => (
              <TabsTrigger className='flex-none' key={value} value={value}>
                {value === 'all' ? t('fleet.filter.all') : t(`nodes.state.${value}`)}
                <span className='font-[family-name:var(--font-data)] text-xs text-[var(--color-text-faint)] tabular-nums'>{loaded ? totals[value] : '—'}</span>
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        {/* Below 48rem the tabs keep the first line and the rest the second; no divider starts a line. */}
        <div className='flex min-w-0 flex-1 items-center gap-2 max-md:basis-full'>
          <StripDivider className='max-md:hidden' />
          {tagChoices.length > 0
            ? (
                <>
                  <SelectField
                    active={tag !== null}
                    aria-label={t('fleet.filter.tag')}
                    className='max-w-48 min-w-0 flex-none'
                    items={[{ label: t('fleet.filter.allTags'), value: ALL_TAGS }, ...tagChoices.map(value => ({ label: value, value }))]}
                    onValueChange={value => setFilter({ tag: value === ALL_TAGS ? null : value })}
                    value={tag ?? ALL_TAGS}
                    variant='flat'
                  />
                  <StripDivider />
                </>
              )
            : null}
          <StripSearch label={t('fleet.search')} onValueChange={value => setFilter({ search: value })} value={search} />
          <StripDivider />
          <StripAction
            aria-pressed={selecting}
            icon={<ListChecks aria-hidden='true' data-icon='inline-start' />}
            label={t('fleet.selectMode')}
            onClick={() => setSelecting(!selecting)}
          />
          <AddNodeAction strip />
        </div>
      </TitleStrip>
      {figuresScroll ? null : figuresCard}
      <div className={figuresScroll ? 'flex min-h-0 flex-1 flex-col' : cn(paneCard, 'min-h-0 flex-1 overflow-hidden')}>{content}</div>
    </section>
  );
}

import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { useShallow } from 'zustand/react/shallow';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Search, SearchX, Server } from 'lucide-react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { Tabs, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { useCallback, useDeferredValue, useEffect, useMemo } from 'react';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

import type { NodeStatus } from '@/utils/fleet-summary';
import type { FleetStateFilter } from '@/stores/fleet-view.store';

import { queries } from '@/api/queries';
import { fleetTags } from '@/utils/panel-commands';
import { fleetFigures } from '@/utils/fleet-summary';
import { TitleStrip } from '@/components/title-strip';
import { useApiClient } from '@/api/api-client-context';
import { SelectField } from '@/components/select-field';
import { useFleetView } from '@/stores/fleet-view.store';
import { LoadingState } from '@/components/loading-state';
import { usePageVisible } from '@/hooks/use-page-visible';
import { AddNodeDialog } from '@/components/add-node-dialog';
import { describeRequestError } from '@/components/error-notice';
import { useCommandSession } from '@/stores/command-session.store';
import { usePrefersReducedMotion } from '@/hooks/use-prefers-reduced-motion';

import { NodePane } from './node-pane';
import { FleetFigures } from './fleet-figures';

const FILTERS = ['all', 'online', 'offline', 'pending'] as const satisfies readonly FleetStateFilter[];
/** Panes that always paint: more than a large window shows at once. */
const EAGER_PANES = 48;
/** The tag filter's value for every tag. */
const ALL_TAGS = '';

const emptyMedia = 'mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6';
const emptyRegion = 'min-h-0 flex-1 rounded-[var(--radius-card)] border border-solid bg-[var(--color-surface-translucent)]';

function counts(items: readonly NodeStatus[]): Record<FleetStateFilter, number> {
  const result = { all: items.length, offline: 0, online: 0, pending: 0 };
  for (const { node } of items) result[node.state] += 1;
  return result;
}

/**
 * Nodes that need attention lead: offline, then over quota, then not
 * enrolled; the rest follow by name. Live load never reorders the panes, so
 * they stay where they are between refreshes.
 */
function attention({ metrics, node }: NodeStatus): number {
  if (node.state === 'offline') return 0;
  if (metrics.quota_exceeded) return 1;
  if (node.state === 'pending') return 2;
  return 3;
}

function matches({ node }: NodeStatus, query: string): boolean {
  return [node.name, node.hostname, node.public_ip ?? '', ...node.tags].some(value => value.toLowerCase().includes(query));
}

/** Escape clears the selection unless a field, a menu, a dialog or the command line has it. */
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

/**
 * The home screen: a title strip that filters the fleet by state and tag and
 * searches it, the fleet's key figures, then every node as a pane in a grid
 * that scrolls in its own region.
 */
export function FleetPage() {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const visible = usePageVisible();
  const reducedMotion = usePrefersReducedMotion();
  const status = useQuery({ ...queries.fleetStatus(client), enabled: visible });
  const update = useQuery(queries.systemUpdate(client));
  const fleetView = useFleetView(useShallow(store => ({
    clearFilters: store.clearFilters,
    search: store.search,
    selected: store.selected,
    setFilter: store.setFilter,
    setVisible: store.setVisible,
    state: store.state,
    tag: store.tag,
    toggle: store.toggle,
  })));
  const { clearFilters, search, selected, setFilter, setVisible, state, tag, toggle } = fleetView;
  const query = useDeferredValue(search.trim().toLowerCase());
  const items = useMemo(() => {
    const names = new Intl.Collator(locale, { numeric: true });
    return [...status.data?.items ?? []]
      .sort((a, b) => attention(a) - attention(b) || names.compare(a.node.name, b.node.name));
  }, [locale, status.data]);
  const totals = useMemo(() => counts(items), [items]);
  const figures = useMemo(() => fleetFigures(items), [items]);
  const tags = useMemo(() => fleetTags(items), [items]);
  const matching = useMemo(() => items.filter(item => (state === 'all' || item.node.state === state)
    && (tag === null || item.node.tags.includes(tag))
    && (query === '' || matches(item, query))), [items, query, state, tag]);
  const order = useMemo(() => matching.map(item => item.node.id), [matching]);
  useEffect(() => setVisible(order), [order, setVisible]);
  useEscapeClearsSelection(selected.size > 0);
  const onToggle = useCallback((id: string, range: boolean) => {
    toggle(id, range ? useFleetView.getState().visible : undefined);
  }, [toggle]);
  const refresh = () => void status.refetch();
  const release = update.data?.current_version ?? null;
  const fleet = items.length > 0;

  let content;
  if (status.isPending) {
    content = <LoadingState label={t('fleet.loading')} fullScreen={false} />;
  } else if (status.data === undefined) {
    content = (
      <Empty className={emptyRegion} role='status'>
        <EmptyHeader>
          <EmptyMedia className={emptyMedia}><SearchX aria-hidden='true' strokeWidth={1.5} /></EmptyMedia>
          <EmptyTitle>{t('nodes.loadFailed')}</EmptyTitle>
          <EmptyDescription>{describeRequestError(status.error)}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent><Button onClick={refresh} variant='outline'>{t('shell.error.retry')}</Button></EmptyContent>
      </Empty>
    );
  } else if (!fleet) {
    content = (
      <Empty className={emptyRegion} role='status'>
        <EmptyHeader>
          <EmptyMedia className={emptyMedia}><Server aria-hidden='true' strokeWidth={1.5} /></EmptyMedia>
          <EmptyTitle>{t('fleet.empty.title')}</EmptyTitle>
        </EmptyHeader>
        <EmptyContent><AddNodeDialog onAdded={refresh} /></EmptyContent>
      </Empty>
    );
  } else if (matching.length === 0) {
    content = (
      <Empty className={emptyRegion} role='status'>
        <EmptyHeader>
          <EmptyMedia className={emptyMedia}><SearchX aria-hidden='true' strokeWidth={1.5} /></EmptyMedia>
          <EmptyTitle>{t('fleet.noMatch.title')}</EmptyTitle>
        </EmptyHeader>
        <EmptyContent><Button onClick={clearFilters} variant='outline'>{t('fleet.noMatch.clear')}</Button></EmptyContent>
      </Empty>
    );
  } else {
    content = (
      <ScrollArea className='min-h-0 flex-1'>
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
              />
            </li>
          ))}
        </ul>
      </ScrollArea>
    );
  }

  // Size containment keeps the panes out of the workspace's height, so the
  // screen fills it exactly and the grid scrolls below the strip and figures.
  return (
    <section className='flex min-h-0 min-w-0 flex-1 flex-col gap-3 [contain:size]'>
      <h1 className='sr-only'>{t('nav.nodes')}</h1>
      <TitleStrip>
        {fleet
          ? (
              <>
                <Tabs className='max-w-full min-w-0' onValueChange={value => setFilter({ state: value as FleetStateFilter })} value={state}>
                  <TabsList aria-label={t('fleet.filter.label')} className='max-w-full overflow-x-auto [scrollbar-width:none]'>
                    {FILTERS.map(value => (
                      <TabsTrigger className='flex-none' key={value} value={value}>
                        {value === 'all' ? t('fleet.filter.all') : t(`nodes.state.${value}`)}
                        <span className='font-[family-name:var(--font-data)] text-xs text-muted-foreground tabular-nums'>{totals[value]}</span>
                      </TabsTrigger>
                    ))}
                  </TabsList>
                </Tabs>
                {tags.length > 0
                  ? (
                      <SelectField
                        aria-label={t('fleet.filter.tag')}
                        className='max-w-48 min-w-32'
                        items={[{ label: t('fleet.filter.allTags'), value: ALL_TAGS }, ...tags.map(value => ({ label: value, value }))]}
                        onValueChange={value => setFilter({ tag: value === ALL_TAGS ? null : value })}
                        value={tag ?? ALL_TAGS}
                      />
                    )
                  : null}
                {/* Search and adding keep a usable width; where the row cannot give it, they take their own line. */}
                <div className='flex min-w-44 flex-1 items-center justify-end gap-2 sm:min-w-80'>
                  <div className='relative min-w-0 flex-1 sm:max-w-64'>
                    <Search aria-hidden='true' className='pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground' />
                    <Input
                      aria-label={t('fleet.search')}
                      className='pl-9'
                      onChange={event => setFilter({ search: event.target.value })}
                      placeholder={t('fleet.search')}
                      type='search'
                      value={search}
                    />
                  </div>
                  <AddNodeDialog iconOnPhone onAdded={refresh} />
                </div>
              </>
            )
          : null}
      </TitleStrip>
      {fleet ? <FleetFigures figures={figures} /> : null}
      <div className='flex min-h-0 flex-1 flex-col'>{content}</div>
    </section>
  );
}

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Search, SearchX, Server } from 'lucide-react';
import { useDeferredValue, useMemo, useState } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

import type { NodeStatus } from '@/utils/fleet-summary';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { LoadingState } from '@/components/loading-state';
import { usePageVisible } from '@/hooks/use-page-visible';
import { AddNodeDialog } from '@/components/add-node-dialog';
import { ShellNavigationTrigger } from '@/components/app-shell';
import { describeRequestError } from '@/components/error-notice';
import { WorkspaceToolbar, WorkspaceToolbarActions } from '@/components/workspace-toolbar';

import { NodeCard } from './node-card';

/** How often the view reads the fleet while it is in view. */
const REFRESH_MS = 5_000;

const FILTERS = ['all', 'online', 'offline', 'pending'] as const;
type Filter = typeof FILTERS[number];

const emptyMedia = 'mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6';
const emptyRegion = 'min-h-0 flex-1 rounded-[var(--radius-card)] border border-solid bg-[var(--color-surface-translucent)]';

function counts(items: readonly NodeStatus[]): Record<Filter, number> {
  const result = { all: items.length, offline: 0, online: 0, pending: 0 };
  for (const { node } of items) result[node.state] += 1;
  return result;
}

/**
 * Nodes that need attention lead: offline, then over quota, then not
 * enrolled; the rest follow by name. Live load never reorders the grid, so
 * cards stay where they are between refreshes.
 */
function attention({ metrics, node }: NodeStatus): number {
  if (node.state === 'offline') return 0;
  if (metrics.quota_exceeded) return 1;
  if (node.state === 'pending') return 2;
  return 3;
}

/**
 * The fleet: every node as a card with its live load. One fixed toolbar row
 * filters by state, with each state's count, then searches and adds a node;
 * the grid scrolls in its own region below it.
 */
export function FleetPage() {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const visible = usePageVisible();
  const status = useQuery({
    ...queries.fleetStatus(client),
    enabled: visible,
    refetchInterval: visible ? REFRESH_MS : false,
  });
  const [search, setSearch] = useState('');
  const query = useDeferredValue(search.trim().toLowerCase());
  const [filter, setFilter] = useState<Filter>('all');
  const items = useMemo(() => {
    const names = new Intl.Collator(locale, { numeric: true });
    return [...status.data?.items ?? []]
      .sort((a, b) => attention(a) - attention(b) || names.compare(a.node.name, b.node.name));
  }, [locale, status.data]);
  const totals = useMemo(() => counts(items), [items]);
  const matching = useMemo(() => items.filter(({ node }) => (filter === 'all' || node.state === filter)
    && (query === '' || [node.name, node.hostname, node.public_ip ?? ''].some(value => value.toLowerCase().includes(query)))), [filter, items, query]);
  const refresh = () => void status.refetch();
  // The row keeps its controls before the first read and while any node exists;
  // an empty fleet leads with adding one instead.
  const controls = status.isPending || items.length > 0;

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
  } else if (items.length === 0) {
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
        <EmptyContent>
          <Button
            onClick={() => {
              setSearch('');
              setFilter('all');
            }}
            variant='outline'
          >
            {t('fleet.noMatch.clear')}
          </Button>
        </EmptyContent>
      </Empty>
    );
  } else {
    content = (
      <ScrollArea className='min-h-0 flex-1'>
        <ul aria-label={t('nav.nodes')} className='grid grid-cols-[repeat(auto-fill,minmax(min(100%,22rem),1fr))] content-start gap-4'>
          {matching.map(item => <li className='flex min-w-0 flex-col *:flex-1' key={item.node.id}><NodeCard item={item} now={status.dataUpdatedAt} /></li>)}
        </ul>
      </ScrollArea>
    );
  }

  // Size containment keeps the cards out of the workspace's height, so the view
  // fills it exactly and the grid scrolls below the fixed toolbar row.
  return (
    <Tabs className='min-h-0 min-w-0 flex-1 gap-[var(--workspace-content-gap)] [contain:size]' onValueChange={value => setFilter(value as Filter)} value={filter}>
      <h1 className='sr-only'>{t('nav.nodes')}</h1>
      <WorkspaceToolbar>
        <ShellNavigationTrigger />
        {controls
          ? (
              // Phones put the search and adding beside the navigation and give the filter its own full row.
              <TabsList aria-label={t('fleet.filter.label')} className='max-sm:order-last max-sm:w-full'>
                {FILTERS.map(value => (
                  <TabsTrigger key={value} value={value}>
                    {value === 'all' ? t('fleet.filter.all') : t(`nodes.state.${value}`)}
                    {status.data ? <span className='font-[family-name:var(--font-data)] text-xs text-muted-foreground tabular-nums'>{totals[value]}</span> : null}
                  </TabsTrigger>
                ))}
              </TabsList>
            )
          : null}
        {controls
          ? (
              <WorkspaceToolbarActions className='max-sm:min-w-0'>
                <div className='relative min-w-0 flex-1 sm:w-64 sm:flex-none'>
                  <Search aria-hidden='true' className='pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground' />
                  <Input
                    aria-label={t('fleet.search')}
                    className='pl-9'
                    onChange={event => setSearch(event.target.value)}
                    placeholder={t('fleet.search')}
                    type='search'
                    value={search}
                  />
                </div>
                <AddNodeDialog onAdded={refresh} />
              </WorkspaceToolbarActions>
            )
          : null}
      </WorkspaceToolbar>
      <TabsContent className='flex min-h-0 flex-col' value={filter}>{content}</TabsContent>
    </Tabs>
  );
}

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { formatBytes } from '@shared/lib/telemetry-format';
import { useDeferredValue, useMemo, useState } from 'react';
import { ArrowDown, ArrowUp, Search, SearchX, Server } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@shared/components/ui/tooltip';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { LoadingState } from '@/components/loading-state';
import { ListPagination } from '@/components/list-pagination';
import { ShellNavigationTrigger } from '@/components/app-shell';
import { describeRequestError } from '@/components/error-notice';

import { NodeCard } from './node-card';
import { AddNodeDialog } from './add-node-dialog';

const emptyMedia = 'mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6';

/** The fleet: every node as a card, with the fleet's online count and summed period usage. */
export function FleetPage() {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const nodes = useQuery({ ...queries.fleetNodes(client), refetchInterval: 15_000 });
  const traffic = useQuery({ ...queries.trafficStatus(client), refetchInterval: 30_000 });
  const [search, setSearch] = useState('');
  const query = useDeferredValue(search.trim().toLowerCase());
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(10);
  const items = useMemo(() => nodes.data?.items ?? [], [nodes.data]);
  const matching = useMemo(() => query === ''
    ? items
    : items.filter(node => [node.name, node.hostname, node.public_ip ?? ''].some(value => value.toLowerCase().includes(query))), [items, query]);
  const pages = Math.max(1, Math.ceil(matching.length / size));
  const current = Math.min(page, pages);
  if (current !== page) setPage(current);
  const visible = matching.slice((current - 1) * size, current * size);
  const usage = new Map(traffic.data?.nodes.map(item => [item.node_id, item]));
  const online = items.filter(node => node.state === 'online').length;
  const refresh = () => {
    void nodes.refetch();
    void traffic.refetch();
  };

  let content;
  if (nodes.isPending) {
    content = <LoadingState label={t('fleet.loading')} fullScreen={false} />;
  } else if (nodes.isError) {
    content = (
      <Empty className='rounded-[var(--radius-card)] border border-solid bg-card' role='status'>
        <EmptyHeader>
          <EmptyMedia className={emptyMedia}><SearchX aria-hidden='true' strokeWidth={1.5} /></EmptyMedia>
          <EmptyTitle>{t('nodes.loadFailed')}</EmptyTitle>
          <EmptyDescription>{describeRequestError(nodes.error)}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent><Button onClick={refresh} variant='outline'>{t('shell.error.retry')}</Button></EmptyContent>
      </Empty>
    );
  } else if (items.length === 0) {
    content = (
      <Empty className='rounded-[var(--radius-card)] border border-solid bg-card' role='status'>
        <EmptyHeader>
          <EmptyMedia className={emptyMedia}><Server aria-hidden='true' strokeWidth={1.5} /></EmptyMedia>
          <EmptyTitle>{t('fleet.empty.title')}</EmptyTitle>
          <EmptyDescription>{t('fleet.empty.description')}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent><AddNodeDialog onAdded={refresh} /></EmptyContent>
      </Empty>
    );
  } else if (matching.length === 0) {
    content = (
      <Empty className='rounded-[var(--radius-card)] border border-solid bg-card' role='status'>
        <EmptyHeader>
          <EmptyMedia className={emptyMedia}><SearchX aria-hidden='true' strokeWidth={1.5} /></EmptyMedia>
          <EmptyTitle>{t('fleet.noMatch.title')}</EmptyTitle>
          <EmptyDescription>{t('fleet.noMatch.description')}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent><Button onClick={() => setSearch('')} variant='outline'>{t('fleet.noMatch.clear')}</Button></EmptyContent>
      </Empty>
    );
  } else {
    content = (
      <>
        <ul aria-label={t('nav.nodes')} className='grid grid-cols-[repeat(auto-fill,minmax(min(100%,18rem),1fr))] content-start gap-4'>
          {visible.map(node => <li className='flex min-w-0 flex-col *:flex-1' key={node.id}><NodeCard node={node} usage={usage.get(node.id)} /></li>)}
        </ul>
        <ListPagination
          onPageChange={setPage}
          onPageSizeChange={(value) => {
            setSize(value);
            setPage(1);
          }}
          page={current}
          pageSize={size}
          pages={pages}
        />
      </>
    );
  }

  const upload = traffic.data?.upload_bytes;
  const download = traffic.data?.download_bytes;
  return (
    <section className='flex min-w-0 flex-1 flex-col gap-[var(--workspace-content-gap)]'>
      <h1 className='sr-only'>{t('nav.nodes')}</h1>
      <div className='flex flex-wrap items-center gap-3'>
        <ShellNavigationTrigger />
        <div className='relative min-w-0 flex-1 sm:w-64 sm:flex-none'>
          <Search aria-hidden='true' className='pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground' />
          <Input
            aria-label={t('fleet.search')}
            className='pl-9'
            onChange={(event) => {
              setSearch(event.target.value);
              setPage(1);
            }}
            placeholder={t('fleet.search')}
            type='search'
            value={search}
          />
        </div>
        <div className='flex min-w-0 items-center gap-4 text-xs text-muted-foreground max-sm:order-last max-sm:w-full'>
          <span className='whitespace-nowrap' role='status'>
            {nodes.data ? t('fleet.online', { count: online, total: items.length }) : '—'}
          </span>
          <Tooltip>
            <TooltipTrigger
              render={(
                <span
                  aria-label={t('fleet.usage', { upload: formatBytes(upload, locale), download: formatBytes(download, locale) })}
                  className='flex items-center gap-2 font-mono whitespace-nowrap tabular-nums outline-none'
                  role='group'
                  tabIndex={0}
                />
              )}
            >
              <span aria-hidden='true' className='flex items-center gap-1'>
                <ArrowUp className='size-3.5 text-chart-2' />
                {formatBytes(upload, locale)}
              </span>
              <span aria-hidden='true' className='flex items-center gap-1'>
                <ArrowDown className='size-3.5 text-primary' />
                {formatBytes(download, locale)}
              </span>
            </TooltipTrigger>
            <TooltipContent>{t('fleet.usageHint')}</TooltipContent>
          </Tooltip>
        </div>
        {items.length > 0 ? <div className='ml-auto'><AddNodeDialog onAdded={refresh} /></div> : null}
      </div>
      {content}
    </section>
  );
}

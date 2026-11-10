import { useMemo } from 'react';
import { SearchX, Server } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Button } from '@shared/components/ui/button';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { LoadingState } from '@/components/loading-state';
import { usePageVisible } from '@/hooks/use-page-visible';
import { AddNodeDialog } from '@/components/add-node-dialog';
import { ShellNavigationTrigger } from '@/components/app-shell';
import { describeRequestError } from '@/components/error-notice';
import { attentionItems, fleetFigures } from '@/utils/fleet-summary';

import { FleetFigures } from './fleet-figures';
import { AttentionList } from './attention-list';
import { NodeLoadTable } from './node-load-table';
import { OverviewRegion } from './overview-region';
import { RecentActivity } from './recent-activity';
import { SubscriptionSummary } from './subscription-summary';

/** How often the overview reads the fleet while it is in view. */
const REFRESH_MS = 5_000;

const emptyMedia = 'mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6';

/**
 * The fleet at a glance: its key figures, every node's load and usage, what
 * needs a look, the panel's newest events and the subscriptions' size. Each
 * region links to the view that manages it.
 */
export function OverviewPage() {
  const { i18n, t } = useTranslation();
  const client = useApiClient();
  const visible = usePageVisible();
  const status = useQuery({
    ...queries.fleetStatus(client),
    enabled: visible,
    refetchInterval: visible ? REFRESH_MS : false,
  });
  const items = useMemo(() => status.data?.items ?? [], [status.data]);
  const figures = useMemo(() => fleetFigures(items), [items]);
  const attention = useMemo(() => attentionItems(items), [items]);
  const time = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? i18n.language, { hour: '2-digit', hourCycle: 'h23', minute: '2-digit', second: '2-digit' });
  const updated = status.dataUpdatedAt ? time.format(status.dataUpdatedAt) : '';
  // A failed refresh keeps the last reading on screen, marked as no longer updating.
  const stale = status.isError && status.data !== undefined;

  let content;
  if (status.isPending) {
    content = <LoadingState fullScreen={false} label={t('overview.loading')} />;
  } else if (status.data === undefined) {
    content = (
      <Empty className='rounded-[var(--radius-card)] border border-solid bg-[var(--color-surface-translucent)]' role='status'>
        <EmptyHeader>
          <EmptyMedia className={emptyMedia}><SearchX aria-hidden='true' strokeWidth={1.5} /></EmptyMedia>
          <EmptyTitle>{t('overview.failed')}</EmptyTitle>
          <EmptyDescription>{describeRequestError(status.error)}</EmptyDescription>
        </EmptyHeader>
        <EmptyContent><Button onClick={() => void status.refetch()} variant='outline'>{t('shell.error.retry')}</Button></EmptyContent>
      </Empty>
    );
  } else if (items.length === 0) {
    // With nothing set up, the overview leads with the next step.
    content = (
      <Empty className='rounded-[var(--radius-card)] border border-solid bg-[var(--color-surface-translucent)]' role='status'>
        <EmptyHeader>
          <EmptyMedia className={emptyMedia}><Server aria-hidden='true' strokeWidth={1.5} /></EmptyMedia>
          <EmptyTitle>{t('fleet.empty.title')}</EmptyTitle>
        </EmptyHeader>
        <EmptyContent><AddNodeDialog onAdded={() => void status.refetch()} /></EmptyContent>
      </Empty>
    );
  } else {
    content = (
      <>
        <FleetFigures figures={figures} />
        {/* The nodes table is the view's main surface and takes the height the other regions leave. */}
        <OverviewRegion
          className='max-h-[30rem] @3xl:max-h-none @3xl:min-h-60 @3xl:flex-1'
          meta={(
            <span className={`truncate font-[family-name:var(--font-data)] text-xs tabular-nums ${stale ? 'text-[var(--color-warning-text)]' : 'text-muted-foreground'}`}>
              {stale ? t('overview.stale', { time: updated }) : t('overview.updated', { time: updated })}
            </span>
          )}
          more={{ label: t('overview.nodes.all'), to: '/nodes' }}
          title={t('overview.nodes.title')}
        >
          <NodeLoadTable items={items} />
        </OverviewRegion>
        {/* Phones stack the regions at their content's height; wider views give them one fixed row. */}
        <div className='grid flex-none grid-cols-1 gap-4 @3xl:grid-cols-2 @5xl:grid-cols-3'>
          <OverviewRegion className='max-h-72 @3xl:h-72' title={t('overview.attention.title')}>
            <AttentionList items={attention} />
          </OverviewRegion>
          <OverviewRegion className='max-h-96 @3xl:h-72 @3xl:max-h-none' more={{ label: t('overview.activity.all'), to: '/logs' }} title={t('overview.activity.title')}>
            <RecentActivity />
          </OverviewRegion>
          <OverviewRegion className='@3xl:col-span-2 @5xl:col-span-1 @5xl:h-72' more={{ label: t('overview.subscriptions.manage'), to: '/subscriptions' }} title={t('overview.subscriptions.title')}>
            <SubscriptionSummary />
          </OverviewRegion>
        </div>
      </>
    );
  }

  return (
    <section className='flex min-w-0 flex-1 flex-col gap-[var(--workspace-content-gap)]'>
      <h1 className='sr-only'>{t('nav.overview')}</h1>
      {/* The overview has no toolbar; on phones its first row opens the navigation. */}
      <div className='flex md:hidden'><ShellNavigationTrigger /></div>
      {content}
    </section>
  );
}

import { useTranslation } from 'react-i18next';
import { CloudOff, SearchX } from 'lucide-react';
import { useEffect, useMemo, useRef } from 'react';
import { Button } from '@shared/components/ui/button';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { buttonVariants } from '@shared/components/ui/button-variants';
import { Link, Outlet, useLocation, useParams } from 'react-router-dom';
import { formatDateTime, formatRelativeTime } from '@shared/lib/format-time';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

import type { FleetNode } from '@/api/api-client';

import { queries } from '@/api/queries';
import { nodePath } from '@/utils/node-path';
import { useApiClient } from '@/api/api-client-context';
import { LoadingState } from '@/components/loading-state';
import { withQueryInvalidation } from '@/api/query-client';
import { describeRequestError } from '@/components/error-notice';
import { CurrentNodeContext } from '@/stores/current-node.store';
import { ApiClientContext } from '@/api/api-client-context/api-client-context';
import { ConfigurationSessionProvider } from '@/stores/configuration-session-provider';

import { NodeStrip } from './node-strip';
import { TelemetryProvider } from './telemetry-provider';

/** What a tab shows in place of its view while the node is offline and cannot answer commands. */
function NodeUnavailable({ node }: { node: FleetNode }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  return (
    <Empty className='min-h-80 rounded-[var(--radius-card)] border border-solid bg-[var(--color-surface-translucent)]' role='status'>
      <EmptyHeader>
        <EmptyMedia className='mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6'>
          <CloudOff aria-hidden='true' strokeWidth={1.5} />
        </EmptyMedia>
        <EmptyTitle>{t('node.unavailable.offlineTitle')}</EmptyTitle>
        <EmptyDescription>
          <span title={formatDateTime(node.last_seen_at, locale)}>
            {t('node.unavailable.offlineDescription', { time: formatRelativeTime(node.last_seen_at, locale) })}
          </span>
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}

function NodeLayout({ node }: { node: FleetNode }) {
  const location = useLocation();
  const scrollRef = useRef<HTMLDivElement>(null);
  const settings = location.pathname === nodePath(node.id, 'settings');
  useEffect(() => {
    if (scrollRef.current) scrollRef.current.scrollTop = 0;
  }, [location.pathname]);
  return (
    <div className='flex min-h-0 min-w-0 flex-1 flex-col gap-3'>
      <NodeStrip node={node} />
      <ScrollArea className='-mx-1 flex-1' viewportClassName='flex flex-col px-1' viewportRef={scrollRef}>
        {node.state === 'online' || settings ? <Outlet /> : <NodeUnavailable node={node} />}
      </ScrollArea>
    </div>
  );
}

/** Scopes the API client, configuration session and telemetry to one node. */
function NodeScope({ node }: { node: FleetNode }) {
  const client = useApiClient();
  const cache = useQueryClient();
  const scoped = useMemo(() => withQueryInvalidation(client.forNode(node.id), cache), [client, cache, node.id]);
  return (
    <CurrentNodeContext value={node}>
      <ApiClientContext value={scoped}>
        <ConfigurationSessionProvider sessionKey={node.id}>
          {node.state === 'online'
            ? <TelemetryProvider><NodeLayout node={node} /></TelemetryProvider>
            : <NodeLayout node={node} />}
        </ConfigurationSessionProvider>
      </ApiClientContext>
    </CurrentNodeContext>
  );
}

/**
 * The node's title strip above the window's view; every view inside reads
 * this node only. The status line lists the windows.
 */
export function NodeWorkspace() {
  const { t } = useTranslation();
  const { nodeId = '' } = useParams();
  const client = useApiClient();
  const nodes = useQuery({ ...queries.fleetNodes(client), refetchInterval: 15_000 });
  const node = nodes.data?.items.find(item => item.id === nodeId);

  if (node !== undefined) return <NodeScope key={node.id} node={node} />;
  if (nodes.isPending) return <LoadingState label={t('shell.loading.page')} fullScreen={false} />;
  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <Empty className='rounded-[var(--radius-card)] border border-solid bg-[var(--color-surface-translucent)]' role='status'>
        <EmptyHeader>
          <EmptyMedia className='mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6'>
            <SearchX aria-hidden='true' strokeWidth={1.5} />
          </EmptyMedia>
          <EmptyTitle>{t(nodes.isError ? 'nodes.loadFailed' : 'node.missing.title')}</EmptyTitle>
          {nodes.isError ? <EmptyDescription>{describeRequestError(nodes.error)}</EmptyDescription> : null}
        </EmptyHeader>
        <EmptyContent className='flex-row justify-center'>
          {nodes.isError
            ? <Button onClick={() => void nodes.refetch()} variant='outline'>{t('shell.error.retry')}</Button>
            : null}
          <Link className={buttonVariants()} to='/nodes'>{t('node.missing.action')}</Link>
        </EmptyContent>
      </Empty>
    </div>
  );
}

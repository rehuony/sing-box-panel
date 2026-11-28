import { useTranslation } from 'react-i18next';
import { CloudOff, SearchX } from 'lucide-react';
import { Button } from '@shared/components/ui/button';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { buttonVariants } from '@shared/components/ui/button-variants';
import { Link, Outlet, useLocation, useParams } from 'react-router-dom';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { formatDateTime, formatRelativeTime } from '@shared/lib/format-time';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

import type { FleetNode, FleetStatus } from '@/api/api-client';

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
  // Size containment keeps the view's content out of the workspace's height, so
  // the strip stays in place and a long view scrolls in its own region below it.
  return (
    <div className='flex min-h-0 min-w-0 flex-1 flex-col gap-[var(--workspace-content-gap)] [contain:size]'>
      <NodeStrip node={node} />
      {/*
        The bleed keeps a focus ring at the view's edge from being clipped. It
        fits only where the shell's viewport carries the edge padding; anywhere
        else it would overflow the shell's scroll area sideways.
      */}
      <ScrollArea className='flex-1 max-sm:-mx-1' viewportClassName='flex flex-col max-sm:px-1' viewportRef={scrollRef}>
        {node.state === 'online' || settings ? <Outlet /> : <NodeUnavailable node={node} />}
      </ScrollArea>
    </div>
  );
}

/** Whether two readings of a node differ only in when it last reported or was seen, which move with every report. */
function sameApartFromReportTimes(first: FleetNode, second: FleetNode): boolean {
  const keys = Object.keys(second) as (keyof FleetNode)[];
  return keys.length === Object.keys(first).length
    && keys.every(key => key === 'last_seen_at' || key === 'reported_at' || first[key] === second[key]);
}

/**
 * Scopes the API client, configuration session and telemetry to one node.
 * The tree stays the same whether the node is online or not, so a view's
 * state, such as an unsaved settings draft, outlives a change of state.
 * The views read the node as it was until something besides its report
 * times changes, so a report re-renders the title strip alone; none of them
 * shows those times, and the strip reads the node as the fleet reports it.
 */
function NodeScope({ node }: { node: FleetNode }) {
  const [viewNode, setViewNode] = useState(node);
  if (viewNode !== node && !sameApartFromReportTimes(viewNode, node)) setViewNode(node);
  const client = useApiClient();
  const cache = useQueryClient();
  const scoped = useMemo(() => withQueryInvalidation(client.forNode(node.id), cache), [client, cache, node.id]);
  // The settings window's form waits for the node's settings; reading them on
  // entering the node lets it open with the window instead of after it.
  useEffect(() => {
    void cache.prefetchQuery(queries.nodeSettings(scoped));
  }, [cache, scoped]);
  return (
    <CurrentNodeContext value={viewNode}>
      <ApiClientContext value={scoped}>
        <ConfigurationSessionProvider sessionKey={node.id}>
          <TelemetryProvider online={node.state === 'online'}><NodeLayout node={node} /></TelemetryProvider>
        </ConfigurationSessionProvider>
      </ApiClientContext>
    </CurrentNodeContext>
  );
}

/**
 * The node's title strip above the window's view; every view inside reads
 * this node only. The node is the one the fleet stream reports, as the status
 * line shows it. The status line lists the windows.
 */
export function NodeWorkspace() {
  const { t } = useTranslation();
  const { nodeId = '' } = useParams();
  const client = useApiClient();
  // The workspace follows its own node, so another node's frame or this one's live metrics re-render nothing here.
  const select = useCallback(
    (status: FleetStatus) => status.items.find(item => item.node.id === nodeId)?.node ?? null,
    [nodeId],
  );
  const nodes = useQuery({ ...queries.fleetStatus(client), select });
  const node = nodes.data;

  if (node) return <NodeScope key={node.id} node={node} />;
  if (nodes.isPending) return <LoadingState label={t('shell.loading.page')} fullScreen={false} />;
  // A fleet that was read but lacks the node no longer has it; without any reading, the read failed.
  const failed = node === undefined;
  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <Empty className='rounded-[var(--radius-card)] border border-solid bg-[var(--color-surface-translucent)]' role='status'>
        <EmptyHeader>
          <EmptyMedia className='mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6'>
            <SearchX aria-hidden='true' strokeWidth={1.5} />
          </EmptyMedia>
          <EmptyTitle>{t(failed ? 'nodes.loadFailed' : 'node.missing.title')}</EmptyTitle>
          {failed ? <EmptyDescription>{describeRequestError(nodes.error)}</EmptyDescription> : null}
        </EmptyHeader>
        <EmptyContent className='flex-row justify-center'>
          {failed
            ? <Button onClick={() => void nodes.refetch()} variant='outline'>{t('shell.error.retry')}</Button>
            : null}
          <Link className={buttonVariants()} to='/nodes'>{t('node.missing.action')}</Link>
        </EmptyContent>
      </Empty>
    </div>
  );
}

import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { CloudOff, KeyRound, SearchX } from 'lucide-react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { buttonVariants } from '@shared/components/ui/button-variants';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, NavLink, Outlet, useLocation, useParams } from 'react-router-dom';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

import type { NodeTab } from '@/utils/node-path';
import type { PagePath } from '@/routes/page-loaders';
import type { FleetNode, NodeEnrollment } from '@/api/api-client';

import { queries } from '@/api/queries';
import { nodePath } from '@/utils/node-path';
import { preloadPage } from '@/routes/page-loaders';
import { useApiClient } from '@/api/api-client-context';
import { LoadingState } from '@/components/loading-state';
import { withQueryInvalidation } from '@/api/query-client';
import { ShellNavigationTrigger } from '@/components/app-shell';
import { CurrentNodeContext } from '@/stores/current-node.store';
import { NodeEnrollmentDialog } from '@/components/node-enrollment-dialog';
import { describeRequestError, ErrorNotice } from '@/components/error-notice';
import { ApiClientContext } from '@/api/api-client-context/api-client-context';
import { formatDateTime, formatRelativeTime } from '@/utils/format-relative-time';
import { ConfigurationSessionProvider } from '@/stores/configuration-session-provider';

import { NodeObjectRow } from './node-object-row';
import { TelemetryProvider } from './telemetry-provider';

const tabs = [
  { tab: undefined, labelKey: 'node.tabs.overview', page: 'node-overview' },
  { tab: 'cores', labelKey: 'node.tabs.cores', page: 'node-cores' },
  { tab: 'configuration', labelKey: 'node.tabs.configuration', page: 'node-configuration' },
  { tab: 'logs', labelKey: 'node.tabs.logs', page: 'node-logs' },
  { tab: 'settings', labelKey: 'node.tabs.settings', page: 'node-settings' },
] as const satisfies readonly { tab: NodeTab | undefined; labelKey: string; page: PagePath }[];

function NodeTabs({ node }: { node: FleetNode }) {
  const { t } = useTranslation();
  return (
    <nav aria-label={t('node.tabs.label')} className='-mb-px flex min-w-0 gap-1 overflow-x-auto [scrollbar-width:none]'>
      {tabs.map(item => (
        <NavLink
          className='relative inline-flex h-[var(--control-height)] shrink-0 items-center rounded-[var(--radius-control)] px-3 text-sm font-medium whitespace-nowrap text-muted-foreground transition-colors duration-[var(--dur-short)] ease-out outline-none after:absolute after:inset-x-3 after:bottom-0 after:h-0.5 after:rounded-full after:bg-foreground after:opacity-0 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:text-foreground aria-[current=page]:after:opacity-100'
          end
          key={item.page}
          onFocus={() => preloadPage(item.page)}
          onPointerEnter={() => preloadPage(item.page)}
          to={nodePath(node.id, item.tab)}
        >
          {t(item.labelKey)}
        </NavLink>
      ))}
    </nav>
  );
}

/** What a tab shows in place of its view while the node cannot answer commands. */
function NodeUnavailable({ node }: { node: FleetNode }) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const client = useApiClient();
  const [enrollment, setEnrollment] = useState<NodeEnrollment | null>(null);
  const issue = useMutation({
    mutationFn: () => client.createNodeEnrollment(node.id),
    onSuccess: setEnrollment,
  });
  const pending = node.state === 'pending';
  return (
    <Empty className='min-h-80 rounded-[var(--radius-card)] border border-solid bg-card' role='status'>
      <EmptyHeader>
        <EmptyMedia className='mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6'>
          {pending ? <KeyRound aria-hidden='true' strokeWidth={1.5} /> : <CloudOff aria-hidden='true' strokeWidth={1.5} />}
        </EmptyMedia>
        <EmptyTitle>{t(pending ? 'node.unavailable.pendingTitle' : 'node.unavailable.offlineTitle')}</EmptyTitle>
        <EmptyDescription>
          {pending
            ? t('node.unavailable.pendingDescription')
            : (
                <span title={formatDateTime(node.last_seen_at, locale)}>
                  {t('node.unavailable.offlineDescription', { time: formatRelativeTime(node.last_seen_at, locale) })}
                </span>
              )}
        </EmptyDescription>
      </EmptyHeader>
      {pending
        ? (
            <EmptyContent>
              <Button disabled={issue.isPending} onClick={() => issue.mutate()}>
                {t(issue.isPending ? 'node.unavailable.issuing' : 'node.unavailable.issue')}
              </Button>
              {issue.error ? <ErrorNotice error={issue.error} title={t('node.unavailable.issueFailed')} /> : null}
            </EmptyContent>
          )
        : null}
      <NodeEnrollmentDialog enrollment={enrollment} onClose={() => setEnrollment(null)} title={t('enrollment.title')} />
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
    <div className='flex min-h-0 min-w-0 flex-1 flex-col gap-2'>
      <div className='flex flex-none flex-col gap-1 border-b'>
        <NodeObjectRow node={node} />
        <NodeTabs node={node} />
      </div>
      <ScrollArea className='-mx-1 flex-1' viewportClassName='flex flex-col px-1 pt-2' viewportRef={scrollRef}>
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

function WorkspaceState({ children }: { children: React.ReactNode }) {
  return (
    <div className='flex min-h-0 flex-1 flex-col gap-4'>
      <div className='flex items-center'><ShellNavigationTrigger /></div>
      {children}
    </div>
  );
}

/** The node's object row and tabs above the tab's view; every view inside reads this node only. */
export function NodeWorkspace() {
  const { t } = useTranslation();
  const { nodeId = '' } = useParams();
  const client = useApiClient();
  const nodes = useQuery({ ...queries.fleetNodes(client), refetchInterval: 15_000 });
  const node = nodes.data?.items.find(item => item.id === nodeId);

  if (node !== undefined) return <NodeScope key={node.id} node={node} />;
  if (nodes.isPending) return <LoadingState label={t('shell.loading.page')} fullScreen={false} />;
  return (
    <WorkspaceState>
      <Empty className='rounded-[var(--radius-card)] border border-solid bg-card' role='status'>
        <EmptyHeader>
          <EmptyMedia className='mb-3 size-12 rounded-[var(--radius-card)] border bg-[var(--color-surface-raised)] text-muted-foreground [&_svg]:size-6'>
            <SearchX aria-hidden='true' strokeWidth={1.5} />
          </EmptyMedia>
          <EmptyTitle>{t(nodes.isError ? 'nodes.loadFailed' : 'node.missing.title')}</EmptyTitle>
          <EmptyDescription>
            {nodes.isError ? describeRequestError(nodes.error) : t('node.missing.description')}
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent className='flex-row justify-center'>
          {nodes.isError
            ? <Button onClick={() => void nodes.refetch()} variant='outline'>{t('shell.error.retry')}</Button>
            : null}
          <Link className={buttonVariants()} to='/nodes'>{t('node.missing.action')}</Link>
        </EmptyContent>
      </Empty>
    </WorkspaceState>
  );
}

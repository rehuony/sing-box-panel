import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { Spinner } from '@shared/components/ui/spinner';
import { ScrollArea } from '@shared/components/ui/scroll-area';

import type { LogLevel } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { usePageVisible } from '@/hooks/use-page-visible';
import { logTitle } from '@/utils/panel-log-presentation';

/** How many of the newest panel events the overview lists; the region scrolls past what fits. */
const LIMIT = 20;

const levelTone: Record<LogLevel, string> = {
  debug: 'text-muted-foreground',
  error: 'text-[var(--color-danger-text)]',
  fatal: 'text-[var(--color-danger-text)]',
  info: 'text-[var(--color-success-text)]',
  trace: 'text-muted-foreground',
  warn: 'text-[var(--color-warning-text)]',
};

/** The panel's newest events, one line each: time, level and what happened. */
export function RecentActivity() {
  const { i18n, t } = useTranslation();
  const client = useApiClient();
  const visible = usePageVisible();
  const logs = useQuery({
    queryKey: ['panelLogs', LIMIT, '', '', 1],
    queryFn: ({ signal }) => client.listPanelLogs({ limit: LIMIT }, signal),
    enabled: visible,
    refetchInterval: visible ? 15_000 : false,
    staleTime: 5_000,
  });
  const time = new Intl.DateTimeFormat(i18n.resolvedLanguage ?? i18n.language, { hour: '2-digit', hourCycle: 'h23', minute: '2-digit' });

  if (logs.isPending) {
    return (
      <div aria-busy='true' className='flex flex-1 items-center justify-center py-3'>
        <Spinner />
      </div>
    );
  }
  const items = logs.data?.items ?? [];
  if (items.length === 0) {
    return (
      <p className='flex flex-1 items-center justify-center px-4 py-3 text-sm text-muted-foreground' role='status'>
        {logs.isError ? t('overview.activity.failed') : t('overview.activity.empty')}
      </p>
    );
  }
  return (
    <ScrollArea className='min-h-0 flex-1'>
      <ol className='divide-y'>
        {items.map((item) => {
          const title = logTitle(item, t);
          return (
            <li className='flex min-w-0 items-center gap-3 px-4 py-2.5 text-sm' key={item.id}>
              <time className='shrink-0 font-[family-name:var(--font-data)] text-xs text-muted-foreground tabular-nums' dateTime={item.time}>
                {time.format(new Date(item.time))}
              </time>
              <span className={`w-[5ch] shrink-0 font-[family-name:var(--font-data)] text-xs ${levelTone[item.level]}`}>{item.level.toUpperCase()}</span>
              <span className='min-w-0 truncate' title={title}>{title}</span>
            </li>
          );
        })}
      </ol>
    </ScrollArea>
  );
}

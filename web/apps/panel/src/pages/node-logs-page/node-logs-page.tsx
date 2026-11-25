import { useTranslation } from 'react-i18next';
import { Tabs, TabsList, TabsTrigger } from '@shared/components/ui/tabs';

import { useHashTab } from '@/hooks/use-hash-tab';
import { LogEntriesView } from '@/components/panel-log';
import { useCurrentNode } from '@/stores/current-node.store';

import { CoreLogView } from './core-log-view';
import { RuntimeStatesView } from './runtime-states-view';

/** The logs window's views of one node: its events in the panel's log, its runtime states and its core log. */
const nodeLogViews = ['events', 'states', 'core'] as const;

/** The node's logs window: one card whose toolbar starts with the views' tabs, kept in the address. */
export function NodeLogsPage() {
  const { t } = useTranslation();
  const node = useCurrentNode();
  const [view, setView] = useHashTab('logs-', nodeLogViews, 'events');
  const tabs = (
    <Tabs className='max-w-full min-w-0 flex-none' onValueChange={value => setView(String(value))} value={view}>
      <TabsList aria-label={t('productLogs.tabs.label')} className='max-w-full overflow-x-auto [scrollbar-width:none]' variant='pill'>
        {nodeLogViews.map(value => (
          <TabsTrigger className='flex-none' key={value} value={value}>{t(`productLogs.tabs.${value}`)}</TabsTrigger>
        ))}
      </TabsList>
    </Tabs>
  );
  return (
    <section className='flex min-h-0 min-w-0 flex-1 flex-col gap-[var(--workspace-content-gap)]'>
      <h1 className='sr-only'>{t('node.tabs.logs')}</h1>
      {view === 'core'
        ? <CoreLogView lead={tabs} />
        : view === 'events'
          ? <LogEntriesView key={node.id} lead={tabs} scope={{ nodeID: node.id }} />
          : <RuntimeStatesView key={node.id} lead={tabs} nodeID={node.id} />}
    </section>
  );
}

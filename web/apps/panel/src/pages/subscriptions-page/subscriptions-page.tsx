import { useState } from 'react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';

import { useHashTab } from '@/hooks/use-hash-tab';
import { StripDivider, TitleStrip } from '@/components/title-strip';
import { WorkspaceToolbarActions } from '@/components/workspace-toolbar';

import { SubscriptionNodePanel } from './subscription-node-panel';
import { SubscriptionTokenPanel } from './subscription-token-panel';
import { SubscriptionSourcePanel } from './subscription-source-panel';
import { SubscriptionChannelPanel } from './subscription-channel-panel';
import { useSubscriptionTokenPage } from './use-subscription-token-page';

const AREAS = ['nodes', 'sources', 'channels', 'tokens'] as const;

/**
 * The subscriptions window: the node pool first, then the sources that feed
 * it, the channels that publish it and the keys that read the channels.
 */
export function SubscriptionsPage() {
  const { t } = useTranslation();
  const [area, selectArea] = useHashTab('subscription-', AREAS, 'nodes');
  const [toolbarTarget, setToolbarTarget] = useState<HTMLDivElement | null>(null);
  // An empty pool leads to adding a source, which the sources tab opens as it appears.
  const [addingSource, setAddingSource] = useState(false);
  const tokenPage = useSubscriptionTokenPage(area === 'tokens');

  // Size containment keeps the areas' content out of the workspace's height, so
  // the window fills it exactly and each area scrolls in its own region.
  return (
    <div className='@container flex w-full min-h-0 min-w-0 flex-1 flex-col [contain:size] *:min-w-0'>
      <h1 className='sr-only'>{t('subscriptions.title')}</h1>
      <Tabs className='min-h-0 flex-1 gap-[var(--workspace-content-gap)]' onValueChange={selectArea} value={area}>
        <TitleStrip>
          <TabsList aria-label={t('subscriptions.tabs.label')} className='max-w-full flex-none justify-start overflow-x-auto [scrollbar-width:none] *:data-[slot=tabs-trigger]:flex-none' variant='pill'>
            {AREAS.map(value => <TabsTrigger key={value} value={value}>{t(`subscriptions.tabs.${value}`)}</TabsTrigger>)}
          </TabsList>
          <StripDivider className='max-sm:hidden' />
          <WorkspaceToolbarActions ref={setToolbarTarget} />
        </TitleStrip>
        {/* The pool's cards sit on the content surface like the nodes home's panes; the tables share one card. */}
        <div className={cn('flex min-h-0 flex-1 flex-col', area !== 'nodes' && 'overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]')}>
          <TabsContent className='min-h-0 flex-1 overflow-auto' value='nodes'>
            <SubscriptionNodePanel
              active={area === 'nodes'}
              onAddSource={() => {
                setAddingSource(true);
                selectArea('sources');
              }}
              toolbarTarget={toolbarTarget}
            />
          </TabsContent>
          <TabsContent className='min-h-0 flex-1 overflow-auto' value='sources'>
            <SubscriptionSourcePanel
              active={area === 'sources'}
              onStarted={() => setAddingSource(false)}
              startCreating={addingSource}
              toolbarTarget={toolbarTarget}
            />
          </TabsContent>
          <TabsContent className='min-h-0 flex-1 overflow-auto' value='channels'><SubscriptionChannelPanel active={area === 'channels'} toolbarTarget={toolbarTarget} /></TabsContent>
          <TabsContent className='min-h-0 flex-1 overflow-auto' value='tokens'><SubscriptionTokenPanel list={tokenPage} active={area === 'tokens'} toolbarTarget={toolbarTarget} /></TabsContent>
        </div>
      </Tabs>
    </div>
  );
}

import { useState } from 'react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';

import { useHashTab } from '@/hooks/use-hash-tab';
import { StripDivider, TitleStrip } from '@/components/title-strip';
import { WorkspaceToolbarActions } from '@/components/workspace-toolbar';

import { RuleSetPanel } from './rule-set-panel';
import { SubscriptionNodePanel } from './subscription-node-panel';
import { SubscriptionTokenPanel } from './subscription-token-panel';
import { SubscriptionChannelPanel } from './subscription-channel-panel';
import { useSubscriptionTokenPage } from './use-subscription-token-page';

const AREAS = ['rules', 'nodes', 'channels', 'tokens'] as const;

/**
 * The subscriptions window, opening on the rule sets channels route by,
 * then the node pool with the sources that feed it, the channels that
 * publish the pool and the keys that read the channels.
 */
export function SubscriptionsPage() {
  const { t } = useTranslation();
  const [area, selectArea] = useHashTab('subscription-', AREAS, 'rules');
  const [toolbarTarget, setToolbarTarget] = useState<HTMLDivElement | null>(null);
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
        {/* The pool and the rule sets lay out their own rail and cards; the tables share one card. */}
        <div className={cn('flex min-h-0 flex-1 flex-col', area !== 'nodes' && area !== 'rules' && 'overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]')}>
          <TabsContent className='min-h-0 flex-1 overflow-auto' value='nodes'>
            <SubscriptionNodePanel active={area === 'nodes'} toolbarTarget={toolbarTarget} />
          </TabsContent>
          <TabsContent className='min-h-0 flex-1 overflow-auto' value='rules'>
            <RuleSetPanel active={area === 'rules'} toolbarTarget={toolbarTarget} />
          </TabsContent>
          <TabsContent className='min-h-0 flex-1 overflow-auto' value='channels'><SubscriptionChannelPanel active={area === 'channels'} toolbarTarget={toolbarTarget} /></TabsContent>
          <TabsContent className='min-h-0 flex-1 overflow-auto' value='tokens'><SubscriptionTokenPanel list={tokenPage} active={area === 'tokens'} toolbarTarget={toolbarTarget} /></TabsContent>
        </div>
      </Tabs>
    </div>
  );
}

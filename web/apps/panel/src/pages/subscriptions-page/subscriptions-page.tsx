import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';

import { useHashTab } from '@/hooks/use-hash-tab';
import { StripDivider, TitleStrip } from '@/components/title-strip';
import { WorkspaceToolbarActions } from '@/components/workspace-toolbar';

import { SubscriptionTokenPanel } from './subscription-token-panel';
import { SubscriptionSourcePanel } from './subscription-source-panel';
import { SubscriptionChannelPanel } from './subscription-channel-panel';
import { useSubscriptionTokenPage } from './use-subscription-token-page';

export function SubscriptionsPage() {
  const { t } = useTranslation();
  const [area, selectArea] = useHashTab('subscription-', ['sources', 'tokens', 'channels'], 'sources');
  const [toolbarTarget, setToolbarTarget] = useState<HTMLDivElement | null>(null);
  const tokenPage = useSubscriptionTokenPage(area === 'tokens');

  return (
    <div className='@container flex w-full min-h-0 min-w-0 flex-1 flex-col *:min-w-0'>
      <h1 className='sr-only'>{t('subscriptions.title')}</h1>
      <Tabs className='min-h-0 flex-1 gap-[var(--workspace-content-gap)]' onValueChange={selectArea} value={area}>
        <TitleStrip>
          <TabsList aria-label={t('subscriptions.tabs.label')} className='max-w-full flex-none justify-start overflow-x-auto [scrollbar-width:none] *:data-[slot=tabs-trigger]:flex-none' variant='pill'>
            <TabsTrigger value='sources'>{t('subscriptions.tabs.sources')}</TabsTrigger>
            <TabsTrigger value='tokens'>{t('subscriptions.tabs.tokens')}</TabsTrigger>
            <TabsTrigger value='channels'>{t('subscriptions.tabs.channels')}</TabsTrigger>
          </TabsList>
          <StripDivider className='max-sm:hidden' />
          <WorkspaceToolbarActions ref={setToolbarTarget} />
        </TitleStrip>
        <div className='flex min-h-0 flex-1 flex-col overflow-hidden rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]'>
          <TabsContent className='min-h-0 flex-1 overflow-auto' value='channels'><SubscriptionChannelPanel active={area === 'channels'} toolbarTarget={toolbarTarget} /></TabsContent>
          <TabsContent className='min-h-0 flex-1 overflow-auto' value='sources'><SubscriptionSourcePanel active={area === 'sources'} toolbarTarget={toolbarTarget} /></TabsContent>
          <TabsContent className='min-h-0 flex-1 overflow-auto' value='tokens'><SubscriptionTokenPanel list={tokenPage} active={area === 'tokens'} toolbarTarget={toolbarTarget} /></TabsContent>
        </div>
      </Tabs>
    </div>
  );
}

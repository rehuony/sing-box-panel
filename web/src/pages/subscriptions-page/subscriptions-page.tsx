import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import { useHashTab } from '@/hooks/use-hash-tab';
import { WorkspaceToolbar } from '@/components/workspace-toolbar';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

import { SubscriptionTokenPanel } from './subscription-token-panel';
import { SubscriptionSourcePanel } from './subscription-source-panel';
import { SubscriptionChannelPanel } from './subscription-channel-panel';
import './subscriptions-page.css';

export function SubscriptionsPage() {
  const { t } = useTranslation();
  const [area, selectArea] = useHashTab('subscription-', ['sources', 'tokens', 'channels'], 'sources');
  const [toolbarTarget, setToolbarTarget] = useState<HTMLDivElement | null>(null);

  return (
    <div className='subscriptions-page panel-page'>
      <h1 className='sr-only'>{t('subscriptions.title')}</h1>
      <Tabs onValueChange={selectArea} value={area}>
        <WorkspaceToolbar>
          <TabsList aria-label={t('subscriptions.tabs.label')} className='subscriptions-tabs'>
            <TabsTrigger value='sources'>{t('subscriptions.tabs.sources')}</TabsTrigger>
            <TabsTrigger value='tokens'>{t('subscriptions.tabs.tokens')}</TabsTrigger>
            <TabsTrigger value='channels'>{t('subscriptions.tabs.channels')}</TabsTrigger>
          </TabsList>
          <div className='workspace-toolbar__actions' ref={setToolbarTarget} />
        </WorkspaceToolbar>
        <TabsContent className='subscriptions-tab-panel' value='channels'><SubscriptionChannelPanel active={area === 'channels'} toolbarTarget={toolbarTarget} /></TabsContent>
        <TabsContent className='subscriptions-tab-panel' value='sources'><SubscriptionSourcePanel active={area === 'sources'} toolbarTarget={toolbarTarget} /></TabsContent>
        <TabsContent className='subscriptions-tab-panel' value='tokens'><SubscriptionTokenPanel active={area === 'tokens'} toolbarTarget={toolbarTarget} /></TabsContent>
      </Tabs>
    </div>
  );
}

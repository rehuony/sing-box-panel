import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

import { SubscriptionTokenPanel } from './subscription-token-panel';
import { SubscriptionSourcePanel } from './subscription-source-panel';
import { SubscriptionChannelPanel } from './subscription-channel-panel';
import './subscriptions-page.css';

type SubscriptionArea = 'channels' | 'sources' | 'tokens';

function initialArea(): SubscriptionArea {
  const hash = typeof window === 'undefined' ? '' : window.location.hash;
  const area = hash.replace('#subscription-', '');
  return ['channels', 'sources', 'tokens'].includes(area)
    ? area as SubscriptionArea
    : 'sources';
}

export function SubscriptionsPage() {
  const { t } = useTranslation();
  const [area, setArea] = useState<SubscriptionArea>(initialArea);

  useEffect(() => {
    const syncAreaFromURL = () => setArea(initialArea());
    window.addEventListener('hashchange', syncAreaFromURL);
    window.addEventListener('popstate', syncAreaFromURL);
    return () => {
      window.removeEventListener('hashchange', syncAreaFromURL);
      window.removeEventListener('popstate', syncAreaFromURL);
    };
  }, []);

  function selectArea(next: string) {
    const selected = next as SubscriptionArea;
    setArea(selected);
    const target = new URL(window.location.href);
    target.hash = `subscription-${selected}`;
    window.history.replaceState(window.history.state, '', target);
  }

  return (
    <div className='subscriptions-page'>
      <header className='subscriptions-page__heading'>
        <h1>{t('subscriptions.title')}</h1>
      </header>

      <Tabs onValueChange={selectArea} value={area}>
        <TabsList aria-label={t('subscriptions.tabs.label')} className='subscriptions-tabs'>
          <TabsTrigger value='sources'>{t('subscriptions.tabs.sources')}</TabsTrigger>
          <TabsTrigger value='tokens'>{t('subscriptions.tabs.tokens')}</TabsTrigger>
          <TabsTrigger value='channels'>{t('subscriptions.tabs.channels')}</TabsTrigger>
        </TabsList>
        <TabsContent className='subscriptions-tab-panel' keepMounted value='channels'><SubscriptionChannelPanel active={area === 'channels'} /></TabsContent>
        <TabsContent className='subscriptions-tab-panel' keepMounted value='sources'><SubscriptionSourcePanel /></TabsContent>
        <TabsContent className='subscriptions-tab-panel' keepMounted value='tokens'><SubscriptionTokenPanel /></TabsContent>
      </Tabs>
    </div>
  );
}

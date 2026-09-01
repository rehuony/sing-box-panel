import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Database, KeyRound, RadioTower, UsersRound } from 'lucide-react';

import { ErrorNotice } from '@/components/error-notice';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

import { useSubscriptionUsers } from './use-subscription-users';
import { SubscriptionUserPanel } from './subscription-user-panel';
import { SubscriptionTokenPanel } from './subscription-token-panel';
import { SubscriptionSourcePanel } from './subscription-source-panel';
import { SubscriptionChannelPanel } from './subscription-channel-panel';
import './subscriptions-page.css';

type SubscriptionArea = 'users' | 'channels' | 'sources' | 'tokens';

function initialArea(): SubscriptionArea {
  const hash = typeof window === 'undefined' ? '' : window.location.hash;
  const area = hash.replace('#subscription-', '');
  return ['users', 'channels', 'sources', 'tokens'].includes(area)
    ? area as SubscriptionArea
    : 'users';
}

export function SubscriptionsPage() {
  const { t } = useTranslation();
  const [area, setArea] = useState<SubscriptionArea>(initialArea);
  const {
    catalog,
    catalogError,
    error: usersError,
    reload: reloadUsers,
    reloadCatalog,
    users,
  } = useSubscriptionUsers();

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
        <p>{t('subscriptions.subtitle')}</p>
      </header>

      {usersError === null
        ? null
        : <ErrorNotice error={usersError} title={t('subscriptions.user.loadFailed')} />}

      <Tabs onValueChange={selectArea} value={area}>
        <TabsList aria-label={t('subscriptions.tabs.label')} className='subscriptions-tabs' variant='line'>
          <TabsTrigger value='users'>
            <UsersRound aria-hidden='true' />
            {t('subscriptions.tabs.users')}
          </TabsTrigger>
          <TabsTrigger value='channels'>
            <RadioTower aria-hidden='true' />
            {t('subscriptions.tabs.channels')}
          </TabsTrigger>
          <TabsTrigger value='sources'>
            <Database aria-hidden='true' />
            {t('subscriptions.tabs.sources')}
          </TabsTrigger>
          <TabsTrigger value='tokens'>
            <KeyRound aria-hidden='true' />
            {t('subscriptions.tabs.tokens')}
          </TabsTrigger>
        </TabsList>
        <TabsContent className='subscriptions-tab-panel' keepMounted value='users'>
          <SubscriptionUserPanel
            catalog={catalog}
            catalogError={catalogError}
            reloadCatalog={reloadCatalog}
            reloadUsers={reloadUsers}
            users={users}
          />
        </TabsContent>
        <TabsContent className='subscriptions-tab-panel' keepMounted value='channels'><SubscriptionChannelPanel users={users} /></TabsContent>
        <TabsContent className='subscriptions-tab-panel' keepMounted value='sources'><SubscriptionSourcePanel /></TabsContent>
        <TabsContent className='subscriptions-tab-panel' keepMounted value='tokens'><SubscriptionTokenPanel users={users} /></TabsContent>
      </Tabs>
    </div>
  );
}

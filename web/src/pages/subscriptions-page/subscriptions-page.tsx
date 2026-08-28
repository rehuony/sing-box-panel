import { PageHeading } from '@/components/page-heading';

import { SubscriptionUserPanel } from './subscription-user-panel';
import { SubscriptionTokenPanel } from './subscription-token-panel';
import { SubscriptionSourcePanel } from './subscription-source-panel';
import { SubscriptionChannelPanel } from './subscription-channel-panel';
import './subscriptions-page.css';

export function SubscriptionsPage() {
  return (
    <div className='page-stack'>
      <PageHeading
        eyebrow='Publication / applied bundle'
        summary='Applied local nodes and versioned third-party sources combine with live user grants, channels and token state.'
        title='Publish only explicit grants.'
      />

      <section className='publication-contract' aria-labelledby='publication-contract-title'>
        <div aria-hidden='true' className='publication-contract__line'>
          <span>Applied bundle</span>
          <span>Channel renderer</span>
          <span>Public token</span>
        </div>
        <div>
          <p className='eyebrow'>Publication boundary</p>
          <h2 id='publication-contract-title'>Edits wait for the next successful apply</h2>
          <p>
            Local nodes come only from the last successful apply. User grants, channels,
            token state and each source current-version pointer take effect immediately.
          </p>
        </div>
      </section>

      <SubscriptionUserPanel />
      <SubscriptionChannelPanel />
      <SubscriptionSourcePanel />
      <SubscriptionTokenPanel />
    </div>
  );
}

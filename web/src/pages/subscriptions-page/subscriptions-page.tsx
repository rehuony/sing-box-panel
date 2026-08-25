import { PageHeading } from '@/components/page-heading';

import { SubscriptionChannelPanel } from './subscription-channel-panel';
import { SubscriptionSourcePanel } from './subscription-source-panel';
import { SubscriptionTokenPanel } from './subscription-token-panel';
import './subscriptions-page.css';

export function SubscriptionsPage() {
  return (
    <div className="page-stack">
      <PageHeading
        eyebrow="Publication / applied bundle"
        summary="Channels, attached sources and access tokens are global controls. Public output stays pinned to the last successfully applied bundle."
        title="Publish only frozen state."
      />

      <section className="publication-contract" aria-labelledby="publication-contract-title">
        <div aria-hidden="true" className="publication-contract__line">
          <span>Applied bundle</span>
          <span>Channel renderer</span>
          <span>Public token</span>
        </div>
        <div>
          <p className="eyebrow">Publication boundary</p>
          <h2 id="publication-contract-title">Edits wait for the next successful apply</h2>
          <p>
            Token revocation and expiry take effect immediately. Channel, source and address
            changes do not leak ahead of the frozen bundle.
          </p>
        </div>
      </section>

      <SubscriptionChannelPanel />
      <SubscriptionSourcePanel />
      <SubscriptionTokenPanel />
    </div>
  );
}

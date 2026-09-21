import type { ReactNode } from 'react';

import { useTranslation } from 'react-i18next';

import type { SubscriptionNodeSummary } from '@/api/api-client';

import { Badge } from '@/components/ui/badge';

import { subscriptionNodeAddress } from './subscription-node-address';

interface Props {
  actions?: ReactNode;
  unavailable?: boolean;
  builtin?: 'direct' | 'reject';
  node?: SubscriptionNodeSummary;
}

export function SubscriptionNodeCardContent({ node, builtin, actions, unavailable }: Props) {
  const { t } = useTranslation();
  const name = node?.name ?? builtin?.toUpperCase() ?? t('channels.missingNode');
  return (
    <>
      <header>
        <span aria-hidden='true' className={`subscription-node-dot${unavailable || (node && !node.available) ? ' is-unavailable' : ''}`} />
        <span className='subscription-node-name' title={name}><span className='truncate'>{name}</span></span>
        {actions}
      </header>
      <div className='subscription-node-card__body' aria-hidden={node?.hidden || undefined}>
        {node && (
          <>
            <Badge className='subscription-node-card__badge' variant='secondary' title={node.source_name}>
              <span className='truncate'>{node.origin === 'source' ? node.source_name : t('subscriptions.nodes.manual')}</span>
            </Badge>
            {node.origin !== 'source' && (
              <Badge className='subscription-node-card__badge' variant='secondary'>
                {t(node.origin === 'local' ? 'subscriptions.nodes.system' : 'subscriptions.nodes.imported')}
              </Badge>
            )}
            <Badge className='subscription-node-card__badge' variant='success'>{node.type}</Badge>
            <Badge className='subscription-node-card__badge' variant='info' title={subscriptionNodeAddress(node)}>
              <span className='truncate'>{subscriptionNodeAddress(node) || t('subscriptions.nodes.hostMissing')}</span>
            </Badge>
            {node.tls && <Badge className='subscription-node-card__badge' variant='success'>{node.reality ? 'Reality' : 'TLS'}</Badge>}
            {node.sni && (
              <Badge className='subscription-node-card__badge' variant='success' title={node.sni}>
                <span className='truncate'>
                  SNI ·
                  {node.sni}
                </span>
              </Badge>
            )}
          </>
        )}
        {builtin && (
          <>
            <Badge className='subscription-node-card__badge' variant='secondary'>{t('channels.builtinNode')}</Badge>
            <Badge className='subscription-node-card__badge' variant={builtin === 'direct' ? 'success' : 'warning'}>{t(`channels.${builtin}`)}</Badge>
          </>
        )}
        {unavailable && <Badge className='subscription-node-card__badge' variant='warning'>{t('channels.unavailable')}</Badge>}
      </div>
    </>
  );
}

import type { ReactNode } from 'react';

import { memo } from 'react';
import { cn } from '@shared/lib/utils';
import { useTranslation } from 'react-i18next';
import { Badge } from '@shared/components/ui/badge';

import type { ChannelRuleGroup, SubscriptionNodeSummary } from '@/api/api-client';

import { subscriptionNodeAddress } from './subscription-node-address';

interface Props {
  actions?: ReactNode;
  unavailable?: boolean;
  disabledReason?: string;
  builtin?: 'direct' | 'reject';
  node?: SubscriptionNodeSummary;
  group?: Pick<ChannelRuleGroup, 'name' | 'type'>;
}

export const SubscriptionNodeCardContent = memo(({
  node, builtin, group, actions, unavailable, disabledReason,
}: Props) => {
  const { t } = useTranslation();
  const name = node?.name ?? group?.name ?? builtin?.toUpperCase() ?? t('channels.missingNode');
  return (
    <>
      <header>
        <span aria-hidden='true' className={cn('mx-[3px] size-1.5 flex-none rounded-full bg-[var(--color-success)] shadow-[0_0_0_3px_var(--color-success-soft)]', (unavailable || (node && !node.available)) && 'bg-[var(--color-text-faint)] shadow-[0_0_0_3px_var(--color-surface-raised)]')} data-node-dot />
        <span className='flex min-h-(--control-height-xs) min-w-0 flex-1 items-center justify-start text-left text-sm font-normal' title={name}><span className='truncate'>{name}</span></span>
        {actions}
      </header>
      <div className='flex min-h-0 w-full min-w-0 flex-1 flex-wrap content-start items-start justify-start gap-1 text-left' aria-hidden={node?.hidden || undefined}>
        {node && (
          <>
            <Badge className='h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal' variant='secondary' title={node.source_name}>
              <span className='truncate'>{node.origin === 'source' ? node.source_name : t('subscriptions.nodes.manual')}</span>
            </Badge>
            {node.origin !== 'source' && (
              <Badge className='h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal' variant='secondary'>
                {t(node.origin === 'local' ? 'subscriptions.nodes.system' : 'subscriptions.nodes.imported')}
              </Badge>
            )}
            <Badge className='h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal' variant='success'>{node.type}</Badge>
            <Badge className='h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal' variant='info' title={subscriptionNodeAddress(node)}>
              <span className='truncate'>{subscriptionNodeAddress(node) || t('subscriptions.nodes.hostMissing')}</span>
            </Badge>
            {node.tls && <Badge className='h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal' variant='success'>{node.reality ? 'Reality' : 'TLS'}</Badge>}
            {node.sni && (
              <Badge className='h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal' variant='success' title={node.sni}>
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
            <Badge className='h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal' variant='secondary'>{t('channels.builtinNode')}</Badge>
            <Badge className='h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal' variant={builtin === 'direct' ? 'success' : 'warning'}>{t(`channels.${builtin}`)}</Badge>
          </>
        )}
        {group && (
          <>
            <Badge className='h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal' variant='secondary'>{t('channels.strategyGroup')}</Badge>
            <Badge className='h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal' variant='info'>{t(`channels.groupTypes.${group.type}`)}</Badge>
          </>
        )}
        {disabledReason && <Badge className='h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal' variant='secondary'>{disabledReason}</Badge>}
        {unavailable && !disabledReason && <Badge className='h-5.5 max-w-full min-w-0 rounded-[var(--radius-control)] px-1.5 font-normal' variant='warning'>{t('channels.unavailable')}</Badge>}
      </div>
    </>
  );
});

import { useTranslation } from 'react-i18next';
import { FileWarning, KeyRound, Layers3, Network, Route, Rss, SearchX, Send } from 'lucide-react';

import { Spinner } from '@/components/ui/spinner';
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';

const stateIcons = {
  sources: Rss,
  nodes: Network,
  candidates: Network,
  groups: Layers3,
  rules: Route,
  keys: KeyRound,
  activeKeys: KeyRound,
  channels: Send,
  search: SearchX,
  preview: FileWarning,
};

export function SubscriptionContentState({ loading = false, kind = 'nodes', title }: {
  loading?: boolean;
  kind?: keyof typeof stateIcons;
  title?: string;
}) {
  const { t } = useTranslation();
  const Icon = stateIcons[kind];
  return (
    <Empty className='subscription-content-state' role={loading ? 'status' : undefined} aria-label={loading ? t('common.loading') : undefined}>
      <EmptyHeader>
        <EmptyMedia variant='icon' aria-hidden='true'>
          {loading ? <Spinner role={undefined} /> : <Icon />}
        </EmptyMedia>
        <EmptyTitle>{loading ? t('common.loading') : title}</EmptyTitle>
        <EmptyDescription>{t(`subscriptions.states.${loading ? 'loading' : kind}`)}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}

import { useTranslation } from 'react-i18next';
import { FileWarning, KeyRound, Layers3, Network, Route, Rss, SearchX, Send } from 'lucide-react';
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

import { LoadingState } from '@/components/loading-state';

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
  if (loading) {
    return <LoadingState className='min-h-40 flex-1' label={t('common.loading')} fullScreen={false} />;
  }
  const Icon = stateIcons[kind];
  return (
    <Empty className='min-h-40 flex-1'>
      <EmptyHeader>
        <EmptyMedia variant='icon' aria-hidden='true'>
          <Icon />
        </EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
        <EmptyDescription>{t(`subscriptions.states.${kind}`)}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}

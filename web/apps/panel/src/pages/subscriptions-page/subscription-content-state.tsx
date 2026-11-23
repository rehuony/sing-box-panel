import type { ReactNode } from 'react';

import { useTranslation } from 'react-i18next';
import { FileWarning, KeyRound, Layers3, Network, Route, Rss, SearchX, Send } from 'lucide-react';
import { Empty, EmptyContent, EmptyHeader, EmptyMedia, EmptyTitle } from '@shared/components/ui/empty';

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

/** A region's loading or empty state, centred in it, with the next step below its title. */
export function SubscriptionContentState({ children, loading = false, kind = 'nodes', title }: {
  loading?: boolean;
  kind?: keyof typeof stateIcons;
  title?: string;
  /** The next step, such as an action that fills the region. */
  children?: ReactNode;
}) {
  const { t } = useTranslation();
  if (loading) {
    return <LoadingState className='min-h-40 flex-1' label={t('common.loading')} fullScreen={false} />;
  }
  const Icon = stateIcons[kind];
  return (
    <Empty className='min-h-40 flex-1' role='status'>
      <EmptyHeader>
        <EmptyMedia variant='icon' aria-hidden='true'>
          <Icon />
        </EmptyMedia>
        <EmptyTitle>{title}</EmptyTitle>
      </EmptyHeader>
      {children ? <EmptyContent>{children}</EmptyContent> : null}
    </Empty>
  );
}

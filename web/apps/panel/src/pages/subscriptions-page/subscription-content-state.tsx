import { useTranslation } from 'react-i18next';
import { FileWarning, KeyRound, Layers3, Network, Route, Rss, SearchX, Send } from 'lucide-react';

import { RegionState } from '@/components/region-state';
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

/** A region's loading or empty state, centred in it: an icon for what the region holds and its title. */
export function SubscriptionContentState({ loading = false, kind = 'nodes', title }: {
  loading?: boolean;
  kind?: keyof typeof stateIcons;
  title?: string;
}) {
  const { t } = useTranslation();
  if (loading) {
    return <LoadingState className='min-h-40 flex-1' label={t('common.loading')} fullScreen={false} />;
  }
  return <RegionState className='flex-1' icon={stateIcons[kind]} title={title} />;
}

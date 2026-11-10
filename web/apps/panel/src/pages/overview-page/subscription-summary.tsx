import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';

function Count({ label, to, value }: { label: string; to: string; value: number | undefined }) {
  return (
    <Link className='flex min-w-0 flex-1 items-center justify-between gap-3 px-4 py-3 text-sm outline-none hover:bg-[var(--color-surface-raised)] focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset' to={to}>
      <span className='truncate text-muted-foreground'>{label}</span>
      <span className='font-[family-name:var(--font-data)] text-base font-semibold tabular-nums'>{value ?? '—'}</span>
    </Link>
  );
}

/** How many sources, keys and channels the subscriptions hold, each opening its tab. */
export function SubscriptionSummary() {
  const { t } = useTranslation();
  const client = useApiClient();
  const sources = useQuery(queries.sources(client));
  const channels = useQuery(queries.channels(client));
  const tokens = useQuery(queries.tokens(client, 1, 1));
  return (
    <div className='flex flex-1 flex-col divide-y'>
      <Count label={t('overview.subscriptions.sources')} to='/subscriptions#sources' value={sources.data?.length} />
      <Count label={t('overview.subscriptions.tokens')} to='/subscriptions#tokens' value={tokens.data?.total} />
      <Count label={t('overview.subscriptions.channels')} to='/subscriptions#channels' value={channels.data?.length} />
    </div>
  );
}

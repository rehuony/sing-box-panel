import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useQuery, useQueryClient } from '@tanstack/react-query';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { usePageVisible } from '@/hooks/use-page-visible';

import { useSavedOrder } from './use-saved-order';

/**
 * Every key in the order the table was dragged into, saved on the server,
 * read while the keys tab shows. The page holds it, so a read in progress
 * carries on across tab switches.
 */
export function useSubscriptionTokens(active: boolean) {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const visible = usePageVisible();
  const { queryKey } = queries.tokens(client);
  const query = useQuery({ ...queries.tokens(client), enabled: active && visible });
  const { move, ordered } = useSavedOrder({
    failed: t('subscriptions.keys.orderSaveFailed'),
    items: query.data,
    queryKey,
    save: ids => client.setSubscriptionTokenOrder(ids),
    stale: t('subscriptions.keys.orderStale'),
  });
  const refresh = useCallback(() => {
    void cache.invalidateQueries({ queryKey: ['tokens'] });
  }, [cache]);
  return { error: query.error, items: ordered, loading: query.isPending, move, refresh };
}

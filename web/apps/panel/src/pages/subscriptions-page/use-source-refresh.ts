import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useQueryClient } from '@tanstack/react-query';
import { toast } from '@shared/components/ui/toast-manager';

import type { SubscriptionSource } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { describeRequestError } from '@/components/error-notice';

/** Refreshes URL sources on request, saying how each went in a toast. */
export function useSourceRefresh() {
  const { t } = useTranslation();
  const client = useApiClient();
  const cache = useQueryClient();
  const [refreshing, setRefreshing] = useState<ReadonlySet<string>>(() => new Set());

  async function refresh(source: SubscriptionSource) {
    if (refreshing.has(source.id)) return;
    setRefreshing(ids => new Set(ids).add(source.id));
    try {
      const result = await client.refreshSubscriptionSource(source.id);
      toast.add({ title: t('subscriptions.sources.refreshed', { count: result.node_count, name: source.name }), type: 'success' });
    } catch (error) {
      toast.add({ title: t('subscriptions.sources.refreshFailed', { name: source.name }), description: describeRequestError(error), type: 'error' });
    } finally {
      setRefreshing((ids) => {
        const next = new Set(ids);
        next.delete(source.id);
        return next;
      });
      void cache.invalidateQueries({ queryKey: ['sources'] });
    }
  }

  return { refresh, refreshing };
}

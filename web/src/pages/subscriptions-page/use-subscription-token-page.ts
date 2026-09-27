import { useCallback, useState } from 'react';
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { usePageVisible } from '@/hooks/use-page-visible';

export function useSubscriptionTokenPage(active: boolean) {
  const client = useApiClient();
  const cache = useQueryClient();
  const visible = usePageVisible();
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const query = useQuery({
    ...queries.tokens(client, page, pageSize), enabled: active && visible, placeholderData: keepPreviousData,
  });
  const total = query.data?.total ?? 0;
  const pages = Math.max(1, Math.ceil(total / pageSize));
  if (query.data && !query.isPlaceholderData && page > pages) setPage(pages);
  const refresh = useCallback(() => {
    void cache.invalidateQueries({ queryKey: ['tokens'] });
  }, [cache]);
  return {
    items: query.data?.items ?? [], page, pageSize, total, pages,
    loading: query.isPending || query.isPlaceholderData,
    refreshing: query.isFetching,
    error: query.error,
    setPage,
    setPageSize: (value: number) => {
      setPageSize(value);
      setPage(1);
    },
    refresh,
  };
}

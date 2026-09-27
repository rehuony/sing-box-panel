import { queryOptions } from '@tanstack/react-query';

import { listInstalledCoreArtifacts } from '@/utils/installed-core-artifacts';

import type { ApiClient, SubscriptionChannelSummary, SubscriptionSourceSummary, SystemStatus } from './api-client';

import { ApiRequestError } from './api-client';

type Platform = NonNullable<SystemStatus['platform']>;

export const queries = {
  settings: (client: ApiClient) => queryOptions({ queryKey: ['settings'], queryFn: ({ signal }) => client.getPanelSettings(signal), staleTime: Infinity, gcTime: 0, refetchOnWindowFocus: false }),
  coreLogFiles: (client: ApiClient) => queryOptions({ queryKey: ['coreLogFiles'], queryFn: ({ signal }) => client.listCoreLogFiles(signal), staleTime: 5000 }),
  system: (client: ApiClient) => queryOptions({ queryKey: ['system'], queryFn: ({ signal }) => client.getSystemStatus(signal), staleTime: 30_000 }),
  context: (client: ApiClient) => queryOptions({ queryKey: ['context'], queryFn: ({ signal }) => client.getDashboardContext(signal), staleTime: 5_000 }),
  runtime: (client: ApiClient) => queryOptions({ queryKey: ['runtime'], queryFn: ({ signal }) => client.getRuntimeStatus(signal), staleTime: 0 }),
  installed: (client: ApiClient, platform: Platform | undefined) => queryOptions({
    queryKey: ['installed', platform],
    queryFn: ({ signal }) => platform ? listInstalledCoreArtifacts(client, platform, signal) : Promise.resolve([]),
    enabled: platform !== undefined, staleTime: 15_000,
  }),
  catalog: (client: ApiClient, platform: Platform | undefined) => queryOptions({
    queryKey: ['catalog', platform], enabled: platform !== undefined, staleTime: 60_000,
    queryFn: async ({ signal }) => {
      try {
        return await client.listCatalogAssets({ architecture: platform?.arch }, signal);
      } catch (error) {
        // Only an uninitialized catalog needs a remote refresh. A network or
        // authentication failure must not turn a navigation read into a write.
        if (!(error instanceof ApiRequestError) || error.code !== 'catalog_not_initialized') throw error;
        await client.refreshCatalog(false, signal);
        return client.listCatalogAssets({ architecture: platform?.arch }, signal);
      }
    },
    select: data => ({
      ...data, assets: data.assets.filter(asset => asset.os === platform?.os && asset.arch === platform.arch),
    }),
  }),
  nodes: (client: ApiClient) => queryOptions({ queryKey: ['nodes'], queryFn: ({ signal }) => client.getSubscriptionNodeCatalog(signal), staleTime: 5_000 }),
  sources: (client: ApiClient) => queryOptions({
    queryKey: ['sources'], staleTime: 5_000,
    queryFn: async ({ signal }) => {
      const items: SubscriptionSourceSummary[] = [];
      let cursor: { created_at: string; id: string } | undefined;
      do {
        const page = await client.listSubscriptionSources({
          limit: 100, beforeID: cursor?.id, beforeTime: cursor?.created_at,
        }, signal);
        items.push(...page.items);
        cursor = page.next;
        signal.throwIfAborted();
      } while (cursor && items.length < 10_000);
      return items;
    },
  }),
  channels: (client: ApiClient) => queryOptions({
    queryKey: ['channels'], staleTime: 5_000,
    queryFn: async ({ signal }) => {
      const items: SubscriptionChannelSummary[] = [];
      let cursor: { created_at: string; id: string } | undefined;
      do {
        const page = await client.listSubscriptionChannels({
          limit: 100, beforeID: cursor?.id, beforeTime: cursor?.created_at,
        }, signal);
        items.push(...page.items);
        cursor = page.next;
        signal.throwIfAborted();
      } while (cursor && items.length < 10_000);
      return items;
    },
  }),
  tokens: (client: ApiClient, page: number, size: number) => queryOptions({ queryKey: ['tokens', page, size], queryFn: ({ signal }) => client.listSubscriptionTokens({ limit: size, offset: (page - 1) * size }, signal), staleTime: 5_000 }),
};

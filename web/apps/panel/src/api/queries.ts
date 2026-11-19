import type { QueryClient } from '@tanstack/react-query';

import { queryOptions } from '@tanstack/react-query';

import { listInstalledCoreArtifacts } from '@/utils/installed-core-artifacts';

import type { ApiClient, FleetStatus, SubscriptionChannelSummary, SubscriptionSource, SystemStatus } from './api-client';

import { ApiRequestError } from './api-client';

type Platform = NonNullable<SystemStatus['platform']>;

/** How many fleet stream frames each cache has applied, so a read can tell that one overtook it. */
const fleetStreamFrames = new WeakMap<QueryClient, number>();

/**
 * Applies a fleet stream frame to the fleet status query. A read of the
 * status that was in flight when the frame arrived is older than the frame,
 * so it keeps what the frame wrote instead of its own answer.
 */
export function applyFleetStreamFrame(cache: QueryClient, update: (current: FleetStatus | undefined) => FleetStatus) {
  fleetStreamFrames.set(cache, (fleetStreamFrames.get(cache) ?? 0) + 1);
  cache.setQueryData<FleetStatus>(['fleetStatus'], update);
}

export const queries = {
  settings: (client: ApiClient) => queryOptions({ queryKey: ['settings'], queryFn: ({ signal }) => client.getPanelSettings(signal), staleTime: Infinity, gcTime: 0, refetchOnWindowFocus: false }),
  fleetNodes: (client: ApiClient) => queryOptions({ queryKey: ['fleetNodes'], queryFn: ({ signal }) => client.listFleetNodes(signal), staleTime: 15_000 }),
  // The fleet stream drives this query. It restarts with every node whenever
  // the page shows again, so focus does not read the status as well; a stream
  // that stays down hands over to a read.
  fleetStatus: (client: ApiClient) => queryOptions({
    queryKey: ['fleetStatus'], staleTime: 5_000, refetchOnWindowFocus: false,
    queryFn: async ({ client: cache, queryKey, signal }) => {
      const frames = fleetStreamFrames.get(cache) ?? 0;
      const status = await client.getFleetStatus(signal);
      if ((fleetStreamFrames.get(cache) ?? 0) === frames) return status;
      return cache.getQueryData<FleetStatus>(queryKey) ?? status;
    },
  }),
  nodeSettings: (client: ApiClient) => queryOptions({ queryKey: ['nodeSettings', client.nodeID], queryFn: ({ signal }) => client.getNodeSettings(signal), staleTime: 0, gcTime: 0, refetchOnWindowFocus: false }),
  systemUpdate: (client: ApiClient) => queryOptions({ queryKey: ['systemUpdate'], queryFn: ({ signal }) => client.getSystemUpdate(signal), staleTime: 0 }),
  // Node-scoped reads carry the node in their key, so nodes never share data.
  coreLogFiles: (client: ApiClient) => queryOptions({ queryKey: ['coreLogFiles', client.nodeID], queryFn: ({ signal }) => client.listCoreLogFiles(signal), staleTime: 5000 }),
  runtime: (client: ApiClient) => queryOptions({ queryKey: ['runtime', client.nodeID], queryFn: ({ signal }) => client.getRuntimeStatus(signal), staleTime: 0 }),
  installed: (client: ApiClient, platform: Platform | undefined) => queryOptions({
    queryKey: ['installed', client.nodeID, platform],
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
  /** Every version the catalog has an archive of, newest first, for installing on several platforms at once. */
  catalogVersions: (client: ApiClient) => queryOptions({
    queryKey: ['catalog', 'versions'], staleTime: 60_000,
    queryFn: async ({ signal }) => {
      try {
        return await client.listCatalogAssets({}, signal);
      } catch (error) {
        if (!(error instanceof ApiRequestError) || error.code !== 'catalog_not_initialized') throw error;
        await client.refreshCatalog(false, signal);
        return client.listCatalogAssets({}, signal);
      }
    },
    select: data => [...new Set(data.assets.map(asset => asset.version))]
      .sort((left, right) => right.localeCompare(left, 'en', { numeric: true })),
  }),
  /** The subscription node pool in its saved order. */
  nodes: (client: ApiClient) => queryOptions({ queryKey: ['nodes'], queryFn: ({ signal }) => client.listSubscriptionNodes(signal), staleTime: 5_000 }),
  sources: (client: ApiClient) => queryOptions({
    queryKey: ['sources'], staleTime: 5_000,
    queryFn: async ({ signal }) => {
      const items: SubscriptionSource[] = [];
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

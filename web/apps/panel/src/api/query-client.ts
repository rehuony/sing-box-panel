import { QueryClient } from '@tanstack/react-query';

import type { ApiClient, SubscriptionNodeCatalog, SubscriptionNodeSummary } from './api-client';

import { ApiRequestError } from './api-client';

const sessions = new WeakMap<QueryClient, number>();
export function resetQuerySession(cache: QueryClient) {
  sessions.set(cache, (sessions.get(cache) ?? 0) + 1);
  cache.clear();
}

export function createQueryClient() {
  return new QueryClient({ defaultOptions: {
    queries: {
      gcTime: 300_000,
      retry: false, // The HTTP boundary retries transient GET failures once.
      refetchOnWindowFocus: true,
      refetchOnReconnect: true,
    },
    mutations: { retry: false },
  } });
}

export function retryableRead(error: unknown): boolean {
  return error instanceof TypeError
    || (error instanceof ApiRequestError && [408, 429, 500, 502, 503, 504].includes(error.status));
}

type QueryRoot
  = | 'catalog' | 'channels' | 'coreLogFiles' | 'installed' | 'nodes'
    | 'runtime' | 'schema' | 'settings' | 'sources' | 'tokens' | 'users';

const runtimeChange: readonly QueryRoot[] = ['runtime', 'nodes', 'installed'];
const coreArtifactChange: readonly QueryRoot[] = ['installed', 'schema'];
const sourceChange: readonly QueryRoot[] = ['sources', 'nodes', 'channels'];
const nodeChange: readonly QueryRoot[] = ['nodes', 'channels'];
const userChange: readonly QueryRoot[] = ['users', 'tokens'];

// Keyed by ApiClient method, so renaming or removing a write is a type error
// here instead of a silently skipped invalidation.
const invalidations: Partial<Record<keyof ApiClient, readonly QueryRoot[]>> = {
  startRuntime: runtimeChange,
  stopRuntime: runtimeChange,
  restartRuntime: runtimeChange,
  rollbackRuntime: runtimeChange,
  activateStartupArtifact: runtimeChange,
  enableCore: runtimeChange,
  disableCore: runtimeChange,
  installCore: coreArtifactChange,
  importCoreArchive: coreArtifactChange,
  removeCoreArtifact: coreArtifactChange,
  refreshCatalog: ['catalog'],
  createSubscriptionSource: sourceChange,
  updateSubscriptionSource: sourceChange,
  deleteSubscriptionSource: sourceChange,
  refreshSubscriptionSource: sourceChange,
  createSubscriptionSourceVersion: sourceChange,
  restoreSubscriptionSourceVersion: sourceChange,
  createSubscriptionNode: nodeChange,
  updateSubscriptionNode: nodeChange,
  deleteSubscriptionNode: nodeChange,
  setSubscriptionNodeOrder: nodeChange,
  setSubscriptionNodeVisibility: nodeChange,
  createSubscriptionChannel: ['channels'],
  updateSubscriptionChannel: ['channels'],
  deleteSubscriptionChannel: ['channels'],
  createSubscriptionToken: ['tokens'],
  deleteSubscriptionToken: ['tokens'],
  revokeSubscriptionToken: ['tokens'],
  rotateSubscriptionToken: ['tokens'],
  setSubscriptionTokenEnabled: ['tokens'],
  createSubscriptionUser: userChange,
  updateSubscriptionUser: userChange,
  deleteSubscriptionUser: userChange,
  replaceSubscriptionUserGrants: userChange,
  clearCoreLog: ['coreLogFiles'],
  deleteCoreLogFile: ['coreLogFiles'],
  savePanelSettings: ['nodes', 'settings'],
};

function affectedQueries(method: string): readonly QueryRoot[] {
  return Object.hasOwn(invalidations, method) ? invalidations[method as keyof ApiClient] ?? [] : [];
}

/**
 * The provider owns this boundary for both HTTP and demo clients. Reads stay
 * transport-only; React queries are the sole owner of retained server data.
 */
export function withQueryInvalidation(client: ApiClient, cache: QueryClient): ApiClient {
  const methods = new Map<PropertyKey, unknown>();
  return new Proxy(client, {
    get(target, property, receiver) {
      const method = Reflect.get(target, property, receiver);
      if (typeof method !== 'function' || typeof property !== 'string') return method;
      if (methods.has(property)) return methods.get(property);
      const keys = affectedQueries(property);
      const session = ['login', 'logout', 'restorePanelBackup'].includes(property);
      if (!keys.length && !session) return method;
      const wrapped = async (...args: unknown[]) => {
        if (session) resetQuerySession(cache);
        const epoch = sessions.get(cache) ?? 0;
        const targets = property === 'refreshCatalog' && args[0] === false ? [] : keys;
        await Promise.all(targets.map(key => cache.cancelQueries({ queryKey: [key] })));
        try {
          const result = await Reflect.apply(method, target, args);
          if ((sessions.get(cache) ?? 0) !== epoch) throw new DOMException('Session changed', 'AbortError');
          if (property === 'setSubscriptionNodeVisibility') {
            cache.setQueryData<SubscriptionNodeCatalog>(['nodes'], current => current && ({
              ...current, nodes: current.nodes.map(node =>
                node.id === (result as SubscriptionNodeSummary).id ? result as SubscriptionNodeSummary : node),
            }));
          }
          return result;
        } catch (error) {
          if (retryableRead(error) && (sessions.get(cache) ?? 0) === epoch) {
            await Promise.all(targets.map(key => cache.invalidateQueries({ queryKey: [key] })));
          }
          throw error;
        } finally {
          // Do not refetch once per item in a batch. The action owner refreshes
          // active lists once; other consumers revalidate on focus/navigation.
          if ((sessions.get(cache) ?? 0) === epoch) {
            await Promise.all(targets.map(key => cache.invalidateQueries({ queryKey: [key], refetchType: 'none' })));
          }
          if (session && (sessions.get(cache) ?? 0) === epoch) resetQuerySession(cache);
        }
      };
      methods.set(property, wrapped);
      return wrapped;
    },
  });
}

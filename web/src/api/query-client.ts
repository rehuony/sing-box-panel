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

function affectedQueries(method: string): string[] {
  if (/^(?:startRuntime|stopRuntime|restartRuntime|rollbackRuntime|activateStartupArtifact|enableCore|disableCore)$/.test(method)) return ['runtime', 'system', 'context', 'nodes', 'installed'];
  if (/^(?:installCore|importCoreArchive|removeCoreArtifact)$/.test(method)) return ['installed', 'schema', 'system', 'context'];
  if (method === 'refreshCatalog') return ['catalog'];
  if (/^(?:create|update|delete|refresh|restore)SubscriptionSource/.test(method)) return ['sources', 'nodes', 'channels'];
  if (/^(?:create|update|delete|set)SubscriptionNode/.test(method)) return ['nodes', 'channels'];
  if (/^(?:create|update|delete)SubscriptionChannel/.test(method)) return ['channels'];
  if (/^(?:create|delete|revoke|rotate|set)SubscriptionToken/.test(method)) return ['tokens'];
  if (/^(?:create|update|delete|replace)SubscriptionUser/.test(method)) return ['users', 'tokens'];
  if (['clearCoreLog', 'deleteCoreLogFile'].includes(method)) return ['coreLogFiles'];
  if (method === 'saveConfigurationFile') return ['context', 'system'];
  if (method === 'savePanelSettings') return ['system', 'context', 'nodes', 'settings'];
  return [];
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

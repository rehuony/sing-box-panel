import type { Session } from '../api-client';

import { createReadCache } from './read-cache';
import { ApiRequestError } from '../api-client';

interface ProblemDetails {
  code?: string;
  title?: string;
  detail?: string;
  fields?: Record<string, string>;
}

export interface SessionPayload {
  csrfToken?: string;
  displayName: string;
}

export interface HttpApiOptions {
  baseUrl?: string;
  fetcher?: typeof fetch;
}

export interface HttpApiContext {
  baseUrl: string;
  fetcher: typeof fetch;
  clearSession: () => void;
  invalidateReadCache: () => void;
  quoteETag: (value: string) => string;
  acceptSession: (payload: SessionPayload) => Session;
  writeHeaders: (headers?: HeadersInit) => HeadersInit;
  writeJSONHeaders: (headers?: HeadersInit) => HeadersInit;
  subscribeSessionInvalidated: (listener: () => void) => () => void;
  buildQuery: (values: Record<string, string | number | boolean | undefined>) => string;
  openEventStream: (fetcher: typeof fetch, url: string, init: RequestInit) => Promise<Response>;
  request: <T>(fetcher: typeof fetch, url: string, init: RequestInit, cacheForMs?: number) => Promise<T>;
}

async function readProblem(response: Response): Promise<ApiRequestError> {
  let problem: ProblemDetails = {};
  try {
    problem = (await response.json()) as ProblemDetails;
  } catch {
    // A proxy or an older server may return an empty/non-JSON error response.
  }
  return new ApiRequestError(
    problem.detail ?? problem.title ?? `Request failed with status ${response.status}`,
    { status: response.status, code: problem.code ?? 'request_failed', fields: problem.fields },
  );
}

export function createHttpApiContext(options: HttpApiOptions): HttpApiContext {
  const baseUrl = (options.baseUrl ?? '/api/v1').replace(/\/$/, '');
  const fetcher = options.fetcher ?? globalThis.fetch.bind(globalThis);
  let csrfToken = '';
  const sessionInvalidatedListeners = new Set<() => void>();
  const readCache = createReadCache();

  const writeHeaders = (headers: HeadersInit = {}): HeadersInit =>
    csrfToken === '' ? headers : { ...headers, 'X-CSRF-Token': csrfToken };

  async function execute(
    requestFetcher: typeof fetch,
    url: string,
    init: RequestInit,
    accept: string,
  ): Promise<Response> {
    const response = await requestFetcher(url, {
      ...init,
      credentials: 'same-origin',
      headers: { Accept: accept, ...init.headers },
    });
    if (!response.ok) {
      if (response.status === 401) {
        csrfToken = '';
        readCache.clear();
        for (const listener of [...sessionInvalidatedListeners]) listener();
      }
      throw await readProblem(response);
    }
    return response;
  }

  return {
    baseUrl,
    fetcher,
    invalidateReadCache: readCache.clear,
    async request<T>(requestFetcher: typeof fetch, url: string, init: RequestInit, cacheForMs?: number): Promise<T> {
      const read = async (signal = init.signal) => {
        const response = await execute(requestFetcher, url, { ...init, signal }, 'application/json');
        if (response.status === 204) return undefined as T;
        return (await response.json()) as T;
      };
      const mutation = init.method !== 'GET' && init.method !== 'HEAD';
      if (mutation) readCache.clear();
      try {
        if (init.method === 'GET' && cacheForMs !== undefined) {
          return await readCache.read(url, read, cacheForMs, init.signal);
        }
        return await read();
      } finally {
        // Invalidate even on ambiguous failures: a write may have reached the server.
        if (mutation) readCache.clear();
      }
    },
    openEventStream(requestFetcher, url, init) {
      return execute(requestFetcher, url, init, 'text/event-stream');
    },
    writeHeaders,
    writeJSONHeaders: (headers: HeadersInit = {}) =>
      writeHeaders({ 'Content-Type': 'application/json', ...headers }),
    quoteETag: (value: string) => `"${value.replaceAll('"', '')}"`,
    buildQuery(values) {
      const query = new URLSearchParams();
      for (const [key, value] of Object.entries(values)) {
        if (value !== undefined && value !== '') query.set(key, String(value));
      }
      const encoded = query.toString();
      return encoded === '' ? '' : `?${encoded}`;
    },
    acceptSession(payload) {
      readCache.clear();
      csrfToken = payload.csrfToken ?? '';
      return { displayName: payload.displayName };
    },
    clearSession() {
      csrfToken = '';
      readCache.clear();
    },
    subscribeSessionInvalidated(listener) {
      sessionInvalidatedListeners.add(listener);
      return () => sessionInvalidatedListeners.delete(listener);
    },
  };
}

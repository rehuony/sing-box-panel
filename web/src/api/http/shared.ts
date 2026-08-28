import type { Session } from '../api-client';

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
  quoteETag: (value: string) => string;
  acceptSession: (payload: SessionPayload) => Session;
  writeHeaders: (headers?: HeadersInit) => HeadersInit;
  writeJSONHeaders: (headers?: HeadersInit) => HeadersInit;
  request: <T>(fetcher: typeof fetch, url: string, init: RequestInit) => Promise<T>;
  buildQuery: (values: Record<string, string | number | boolean | undefined>) => string;
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

  const writeHeaders = (headers: HeadersInit = {}): HeadersInit =>
    csrfToken === '' ? headers : { ...headers, 'X-CSRF-Token': csrfToken };

  return {
    baseUrl,
    fetcher,
    async request<T>(requestFetcher: typeof fetch, url: string, init: RequestInit): Promise<T> {
      const response = await requestFetcher(url, {
        ...init,
        credentials: 'same-origin',
        headers: { Accept: 'application/json', ...init.headers },
      });
      if (!response.ok) throw await readProblem(response);
      if (response.status === 204) return undefined as T;
      return (await response.json()) as T;
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
      csrfToken = payload.csrfToken ?? '';
      return { displayName: payload.displayName };
    },
    clearSession() {
      csrfToken = '';
    },
  };
}

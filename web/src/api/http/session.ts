import type { HttpApiContext, SessionPayload } from './shared';
import type { ApiClient, DashboardContext } from '../api-client';

import { ApiRequestError } from '../api-client';

export function createSessionHttpApi(context: HttpApiContext) {
  const {
    acceptSession, baseUrl, clearSession, fetcher, request, writeHeaders,
  } = context;
  return {
    async getSession(signal) {
      try {
        const payload = await request<SessionPayload>(fetcher, `${baseUrl}/auth/session`, {
          method: 'GET',
          signal,
        });
        return acceptSession(payload);
      } catch (error) {
        if (error instanceof ApiRequestError && error.status === 401) {
          clearSession();
          return null;
        }
        throw error;
      }
    },
    async login(token, signal) {
      const payload = await request<SessionPayload>(fetcher, `${baseUrl}/auth/session`, {
        method: 'POST',
        body: JSON.stringify({ token }),
        headers: {
          'Content-Type': 'application/json',
        },
        signal,
      });
      return acceptSession(payload);
    },
    async logout(signal) {
      await request<void>(fetcher, `${baseUrl}/auth/session`, {
        method: 'DELETE',
        headers: writeHeaders(),
        signal,
      });
      clearSession();
    },
    getDashboardContext(signal) {
      return request<DashboardContext>(fetcher, `${baseUrl}/dashboard/context`, {
        method: 'GET',
        signal,
      });
    },

  } satisfies Partial<ApiClient>;
}

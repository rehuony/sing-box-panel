import type { HttpApiContext, SessionPayload } from './shared';
import type { ApiClient, DashboardContext, SystemStatus } from '../api-client';

import { ApiRequestError } from '../api-client';
import { loginFingerprint } from './login-fingerprint';

export function createSessionHttpApi(context: HttpApiContext) {
  const {
    acceptSession, baseUrl, clearSession, fetcher, request,
    subscribeSessionInvalidated, writeHeaders,
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
    async login(input, signal) {
      signal?.throwIfAborted();
      const fingerprint = await loginFingerprint();
      signal?.throwIfAborted();
      const payload = await request<SessionPayload>(fetcher, `${baseUrl}/auth/session`, {
        method: 'POST',
        body: JSON.stringify(input),
        headers: {
          'Content-Type': 'application/json',
          ...(fingerprint && { 'X-Client-Fingerprint': fingerprint }),
        },
        signal,
      });
      return acceptSession(payload);
    },
    async logout(signal) {
      try {
        await request<void>(fetcher, `${baseUrl}/auth/session`, {
          method: 'DELETE',
          headers: writeHeaders(),
          signal,
        });
        clearSession();
      } catch (error) {
        if (error instanceof ApiRequestError && error.status === 401) {
          clearSession();
          return;
        }
        throw error;
      }
    },
    subscribeSessionInvalidated,
    getSystemStatus(signal) {
      return request<SystemStatus>(fetcher, `${baseUrl}/system/status`, {
        method: 'GET',
        signal,
      });
    },
    getDashboardContext(signal) {
      return request<DashboardContext>(fetcher, `${baseUrl}/dashboard/context`, {
        method: 'GET',
        signal,
      });
    },

  } satisfies Partial<ApiClient>;
}

import type { ReactNode } from 'react';

import { useEffect, useMemo, useState } from 'react';

import type { Session } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';

import type { AuthSessionValue, AuthStatus } from './auth-session.store';

import { AuthSessionContext } from './auth-session.store';

export interface AuthSessionProviderProps {
  children: ReactNode;
}

export function AuthSessionProvider({ children }: AuthSessionProviderProps) {
  const client = useApiClient();
  const [session, setSession] = useState<Session | null>(null);
  const [status, setStatus] = useState<AuthStatus>('checking');

  useEffect(() => {
    const controller = new AbortController();

    void client
      .getSession(controller.signal)
      .then((nextSession) => {
        if (controller.signal.aborted) {
          return;
        }
        setSession(nextSession);
        setStatus(nextSession === null ? 'anonymous' : 'authenticated');
      })
      .catch((error: unknown) => {
        if (
          controller.signal.aborted
          || (error instanceof DOMException && error.name === 'AbortError')
        ) {
          return;
        }
        setSession(null);
        setStatus('anonymous');
      });

    return () => controller.abort();
  }, [client]);

  const value = useMemo<AuthSessionValue>(
    () => ({
      session,
      status,
      async login(token, signal) {
        const nextSession = await client.login(token, signal);
        setSession(nextSession);
        setStatus('authenticated');
      },
      async logout(signal) {
        await client.logout(signal);
        setSession(null);
        setStatus('anonymous');
      },
    }),
    [client, session, status],
  );

  return <AuthSessionContext value={value}>{children}</AuthSessionContext>;
}

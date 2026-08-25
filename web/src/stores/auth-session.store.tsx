import {
  createContext,
  type ReactNode,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react';

import { useApiClient } from '@/api/api-client-context';
import type { Session } from '@/api/api-client';

type AuthStatus = 'checking' | 'anonymous' | 'authenticated';

interface AuthSessionValue {
  session: Session | null;
  status: AuthStatus;
  login(token: string, signal?: AbortSignal): Promise<void>;
  logout(signal?: AbortSignal): Promise<void>;
}

const AuthSessionContext = createContext<AuthSessionValue | null>(null);

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
          controller.signal.aborted ||
          (error instanceof DOMException && error.name === 'AbortError')
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

  return (
    <AuthSessionContext.Provider value={value}>
      {children}
    </AuthSessionContext.Provider>
  );
}

export function useAuthSession(): AuthSessionValue {
  const value = useContext(AuthSessionContext);

  if (value === null) {
    throw new Error('useAuthSession must be used within AuthSessionProvider');
  }

  return value;
}

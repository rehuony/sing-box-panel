import { createContext, use } from 'react';

import type { Session } from '@/api/api-client';

export type AuthStatus = 'checking' | 'unavailable' | 'anonymous' | 'authenticated';

export interface AuthSessionValue {
  status: AuthStatus;
  session: Session | null;
  retrySession: () => void;
  logout: (signal?: AbortSignal) => Promise<void>;
  login: (token: string, signal?: AbortSignal) => Promise<void>;
}

export const AuthSessionContext = createContext<AuthSessionValue | null>(null);

export function useAuthSession(): AuthSessionValue {
  const value = use(AuthSessionContext);

  if (value === null) {
    throw new Error('useAuthSession must be used within AuthSessionProvider');
  }

  return value;
}

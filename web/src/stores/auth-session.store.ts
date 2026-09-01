import { createContext, use } from 'react';

import type { Session } from '@/api/api-client';

import i18n from '@/i18n';

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
    throw new Error(i18n.t('common.providerRequired', {
      hook: 'useAuthSession',
      provider: 'AuthSessionProvider',
    }));
  }

  return value;
}

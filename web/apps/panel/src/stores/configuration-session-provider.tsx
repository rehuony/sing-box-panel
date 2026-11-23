import type { ReactNode } from 'react';

import { use, useState } from 'react';

import type { ConfigurationSessionStore } from './configuration-session.store';

import {
  ConfigurationSessionContext,
  ConfigurationSessionsContext,
  createConfigurationSessionStore,
} from './configuration-session.store';

/** Holds each node's configuration session until sign-out, so leaving a node keeps its draft. */
export function ConfigurationSessionsProvider({ children }: { children: ReactNode }) {
  const [sessions] = useState(() => new Map<string, ConfigurationSessionStore>());
  return <ConfigurationSessionsContext value={sessions}>{children}</ConfigurationSessionsContext>;
}

/**
 * Provides one configuration session. With a session key inside
 * ConfigurationSessionsProvider it reuses that key's session; otherwise the
 * session lasts as long as this provider.
 */
export function ConfigurationSessionProvider({ children, sessionKey }: { children: ReactNode; sessionKey?: string }) {
  const sessions = use(ConfigurationSessionsContext);
  const [store] = useState(() => {
    if (sessions === null || sessionKey === undefined) return createConfigurationSessionStore();
    const existing = sessions.get(sessionKey) ?? createConfigurationSessionStore();
    sessions.set(sessionKey, existing);
    return existing;
  });

  return <ConfigurationSessionContext value={store}>{children}</ConfigurationSessionContext>;
}

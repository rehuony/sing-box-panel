import type { ReactNode } from 'react';

import { useState } from 'react';

import { ConfigurationSessionContext, createConfigurationSessionStore } from './configuration-session.store';

/** Session memory survives route changes, and is destroyed on sign-out. */
export function ConfigurationSessionProvider({ children }: { children: ReactNode }) {
  const [store] = useState(createConfigurationSessionStore);

  return <ConfigurationSessionContext value={store}>{children}</ConfigurationSessionContext>;
}

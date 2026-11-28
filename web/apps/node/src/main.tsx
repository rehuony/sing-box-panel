import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { CSPProvider } from '@shared/components/ui/csp-provider';

import { App } from '@/app';
import i18n, { i18nReady } from '@/i18n';
import '@shared/styles/global.css';

import { applyThemePreference, storedThemePreference } from '@/hooks/use-theme-preference';

// Paint the remembered theme before the first render; the frame takes over from there.
applyThemePreference(storedThemePreference())();

// The first render waits for the language's translations, so no key shows in their place.
void i18nReady.then(() => {
  const rootElement = document.getElementById('root');
  if (rootElement === null) {
    throw new Error(i18n.t('bootstrap.rootMissing'));
  }
  createRoot(rootElement).render(
    <StrictMode>
      <CSPProvider><App /></CSPProvider>
    </StrictMode>,
  );
});

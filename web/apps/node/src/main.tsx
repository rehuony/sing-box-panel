import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import i18n from '@/i18n';
import { App } from '@/app';
import '@shared/styles/global.css';

import { applyThemePreference, storedThemePreference } from '@/hooks/use-theme-preference';

const rootElement = document.getElementById('root');
if (rootElement === null) {
  throw new Error(i18n.t('bootstrap.rootMissing'));
}

// Paint the remembered theme before the first render; the frame takes over from there.
applyThemePreference(storedThemePreference())();

createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

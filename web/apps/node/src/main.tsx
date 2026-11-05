import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { applyThemeToDocument, resolveTheme, SYSTEM_THEME_QUERY } from '@shared/theme/theme';

import i18n from '@/i18n';
import { App } from '@/app';
import '@shared/styles/global.css';

const rootElement = document.getElementById('root');
if (rootElement === null) {
  throw new Error(i18n.t('bootstrap.rootMissing'));
}

// The console follows the system's light or dark appearance.
const systemTheme = window.matchMedia(SYSTEM_THEME_QUERY);
function applySystemTheme() {
  applyThemeToDocument(
    document.documentElement,
    'system',
    resolveTheme('system', systemTheme.matches),
    document.querySelector<HTMLMetaElement>('meta[name="theme-color"]'),
  );
}
applySystemTheme();
systemTheme.addEventListener('change', applySystemTheme);

createRoot(rootElement).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

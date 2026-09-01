import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import i18n from '@/i18n';
import { App } from '@/app';
import { createBrowserApiClient } from '@/api/browser-api-client';
import '@/styles/global.css';

const rootElement = document.getElementById('root');
const configuredBasePath
  = document
    .querySelector<HTMLMetaElement>('meta[name="sing-box-panel-base-path"]')
    ?.content
    .replace(/\/$/, '') ?? '';
const basePath = configuredBasePath === '__SBP_BASE_PATH__' ? '' : configuredBasePath;

if (rootElement === null) {
  throw new Error(i18n.t('bootstrap.rootMissing'));
}
const applicationRoot = rootElement;

async function bootstrap() {
  const apiClient = await createBrowserApiClient(basePath);

  createRoot(applicationRoot).render(
    <StrictMode>
      <App apiClient={apiClient} basePath={basePath} />
    </StrictMode>,
  );
}

void bootstrap();

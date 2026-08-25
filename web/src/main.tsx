import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import { createHttpApiClient } from '@/api/http-api-client';
import { App } from '@/app';
import '@/styles/global.css';

const rootElement = document.getElementById('root');
const configuredBasePath =
  document
    .querySelector<HTMLMetaElement>('meta[name="sing-box-panel-base-path"]')
    ?.content.replace(/\/$/, '') ?? '';
const basePath = configuredBasePath === '__SBP_BASE_PATH__' ? '' : configuredBasePath;

if (rootElement === null) {
  throw new Error('Root element was not found');
}

createRoot(rootElement).render(
  <StrictMode>
    <App
      apiClient={createHttpApiClient({ baseUrl: `${basePath}/api/v1` })}
      basePath={basePath}
    />
  </StrictMode>,
);

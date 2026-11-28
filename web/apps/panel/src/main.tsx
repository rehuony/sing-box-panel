import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { createBrowserRouter } from 'react-router-dom';
import { CSPProvider } from '@shared/components/ui/csp-provider';

import { App } from '@/app';
import i18n, { i18nReady } from '@/i18n';
import { AppRoutes } from '@/routes/app.routes';
import { RootRouteError } from '@/components/route-error';
import { createHttpApiClient } from '@/api/http-api-client';
import { UnsavedChangesProvider } from '@/stores/unsaved-changes-provider';
import '@shared/styles/global.css';

const rootElement = document.getElementById('root');
const configuredBasePath
  = document
    .querySelector<HTMLMetaElement>('meta[name="sing-box-panel-base-path"]')
    ?.content
    .replace(/\/$/, '') ?? '';
const basePath = configuredBasePath === '__SBP_BASE_PATH__' ? '' : configuredBasePath;

function bootstrap() {
  if (rootElement === null) {
    throw new Error(i18n.t('bootstrap.rootMissing'));
  }
  const apiClient = createHttpApiClient({ baseUrl: `${basePath}/api/v1` });
  const router = createBrowserRouter([
    {
      path: '*',
      element: <UnsavedChangesProvider><AppRoutes /></UnsavedChangesProvider>,
      // Failures inside the shell stay in its view; this one catches the rest.
      errorElement: <RootRouteError />,
    },
  ], { basename: basePath || undefined });

  createRoot(rootElement).render(
    <StrictMode>
      <CSPProvider><App apiClient={apiClient} router={router} /></CSPProvider>
    </StrictMode>,
  );
}

// The first render waits for the language's translations, so no key shows in their place.
void i18nReady.then(bootstrap);

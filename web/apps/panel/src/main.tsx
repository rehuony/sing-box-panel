import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { createBrowserRouter } from 'react-router-dom';
import { CSPProvider } from '@shared/components/ui/csp-provider';

import i18n from '@/i18n';
import { App } from '@/app';
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

if (rootElement === null) {
  throw new Error(i18n.t('bootstrap.rootMissing'));
}
const applicationRoot = rootElement;

function bootstrap() {
  const apiClient = createHttpApiClient({ baseUrl: `${basePath}/api/v1` });
  const router = createBrowserRouter([
    {
      path: '*',
      element: <UnsavedChangesProvider><AppRoutes /></UnsavedChangesProvider>,
      // Failures inside the shell stay in its view; this one catches the rest.
      errorElement: <RootRouteError />,
    },
  ], { basename: basePath || undefined });

  createRoot(applicationRoot).render(
    <StrictMode>
      <CSPProvider><App apiClient={apiClient} router={router} /></CSPProvider>
    </StrictMode>,
  );
}

bootstrap();

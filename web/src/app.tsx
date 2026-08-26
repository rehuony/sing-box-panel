import { BrowserRouter } from 'react-router-dom';

import type { ApiClient } from '@/api/api-client';

import { AppRoutes } from '@/routes';
import { ApiClientProvider } from '@/api/api-client-context';
import { AuthSessionProvider } from '@/stores/auth-session-provider';

export interface AppProps {
  basePath?: string;
  apiClient: ApiClient;
}

export function App({ apiClient, basePath }: AppProps) {
  return (
    <ApiClientProvider client={apiClient}>
      <AuthSessionProvider>
        <BrowserRouter basename={basePath || undefined}>
          <AppRoutes />
        </BrowserRouter>
      </AuthSessionProvider>
    </ApiClientProvider>
  );
}

import { BrowserRouter } from 'react-router-dom';

import type { ApiClient } from '@/api/api-client';
import { ApiClientProvider } from '@/api/api-client-context';
import { AppRoutes } from '@/routes';
import { AuthSessionProvider } from '@/stores/auth-session.store';

export interface AppProps {
  apiClient: ApiClient;
  basePath?: string;
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

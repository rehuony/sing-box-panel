import { BrowserRouter } from 'react-router-dom';

import type { ApiClient } from '@/api/api-client';

import { ThemeProvider } from '@/theme';
import { Toaster } from '@/components/ui/toast';
import { AppRoutes } from '@/routes/app.routes';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ApiClientProvider } from '@/api/api-client-context';
import { AuthSessionProvider } from '@/stores/auth-session-provider';

export interface AppProps {
  basePath?: string;
  apiClient: ApiClient;
}

export function App({ apiClient, basePath }: AppProps) {
  return (
    <ApiClientProvider client={apiClient}>
      <ThemeProvider>
        <TooltipProvider delay={800}>
          <Toaster>
            <AuthSessionProvider>
              <BrowserRouter basename={basePath || undefined}>
                <AppRoutes />
              </BrowserRouter>
            </AuthSessionProvider>
          </Toaster>
        </TooltipProvider>
      </ThemeProvider>
    </ApiClientProvider>
  );
}

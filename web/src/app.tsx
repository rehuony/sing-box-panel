import type { ComponentProps } from 'react';

import { RouterProvider } from 'react-router-dom';

import type { ApiClient } from '@/api/api-client';

import { ThemeProvider } from '@/theme';
import { Toaster } from '@/components/ui/toast';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ApiClientProvider } from '@/api/api-client-context';
import { AuthSessionProvider } from '@/stores/auth-session-provider';

export interface AppProps {
  apiClient: ApiClient;
  router: ComponentProps<typeof RouterProvider>['router'];
}

export function App({ apiClient, router }: AppProps) {
  return (
    <ApiClientProvider client={apiClient}>
      <ThemeProvider>
        <TooltipProvider delay={800}>
          <Toaster>
            <AuthSessionProvider>
              <RouterProvider router={router} />
            </AuthSessionProvider>
          </Toaster>
        </TooltipProvider>
      </ThemeProvider>
    </ApiClientProvider>
  );
}

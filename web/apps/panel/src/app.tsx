import type { ComponentProps } from 'react';

import { RouterProvider } from 'react-router-dom';
import { Toaster } from '@shared/components/ui/toast';
import { TooltipProvider } from '@shared/components/ui/tooltip';

import type { ApiClient } from '@/api/api-client';

import { ThemeProvider } from '@/theme';
import { Backdrop } from '@/components/backdrop';
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
        <TooltipProvider delay={200} closeDelay={100}>
          <Toaster>
            <AuthSessionProvider>
              <Backdrop><RouterProvider router={router} /></Backdrop>
            </AuthSessionProvider>
          </Toaster>
        </TooltipProvider>
      </ThemeProvider>
    </ApiClientProvider>
  );
}

import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { Toaster } from '@shared/components/ui/toast';
import { TooltipProvider } from '@shared/components/ui/tooltip';
import { render, screen, within } from '@testing-library/react';

import { ThemeProvider } from '@/theme';
import { AppRoutes } from '@/routes/app.routes';
import '@/i18n';
import { pageLoaders } from '@/routes/page-loaders';
import { ApiClientProvider } from '@/api/api-client-context';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { createMockApiClient } from '@/tests/api/mock-api-client';
import { AuthSessionProvider } from '@/stores/auth-session-provider';

// A lazy route that failed stays failed for its module, so this file holds the only route that fails.
describe('view errors', () => {
  it('shows a view whose module failed inside the shell, and leaves it with the next window', async () => {
    const user = userEvent.setup();
    vi.spyOn(pageLoaders, 'logs').mockRejectedValue(new TypeError('Failed to fetch dynamically imported module'));
    // React reports the error the boundary caught.
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    render(
      <Toaster>
        <ApiClientProvider client={createMockApiClient()}>
          <ThemeProvider>
            <TooltipProvider delay={0}>
              <AuthSessionProvider>
                <MemoryRouter initialEntries={['/logs']}><AppRoutes /></MemoryRouter>
              </AuthSessionProvider>
            </TooltipProvider>
          </ThemeProvider>
        </ApiClientProvider>
      </Toaster>,
    );
    const failure = await screen.findByRole('alert');
    expect(failure).toHaveTextContent('This view could not be loaded.');
    expect(within(failure).getByRole('button', { name: 'Reload' })).toBeInTheDocument();
    const windows = screen.getByRole('navigation', { name: 'Windows' });
    await user.click(within(windows).getByRole('link', { name: '2: Subscriptions' }));
    expect(await screen.findByRole('heading', { name: 'Subscriptions' })).toBeInTheDocument();
    expect(screen.queryByText('This view could not be loaded.')).not.toBeInTheDocument();
  });
});

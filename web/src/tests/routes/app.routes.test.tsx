import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { act, render, screen, waitFor } from '@testing-library/react';

import '@/i18n';
import { ThemeProvider } from '@/theme';
import { AppRoutes } from '@/routes/app.routes';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ApiClientProvider } from '@/api/api-client-context';
import { AuthSessionProvider } from '@/stores/auth-session-provider';
import { createMockApiClient, testSession } from '@/tests/api/mock-api-client';

function LocationProbe() {
  const location = useLocation();
  return (
    <output aria-label='Current route'>{`${location.pathname}${location.search}${location.hash}`}</output>
  );
}

function renderRoutes(initialEntry: string, client = createMockApiClient()) {
  return render(
    <ApiClientProvider client={client}>
      <ThemeProvider>
        <TooltipProvider delay={0}>
          <AuthSessionProvider>
            <MemoryRouter initialEntries={[initialEntry]}>
              <AppRoutes />
              <LocationProbe />
            </MemoryRouter>
          </AuthSessionProvider>
        </TooltipProvider>
      </ThemeProvider>
    </ApiClientProvider>,
  );
}

async function openSignOut(user: ReturnType<typeof userEvent.setup>) {
  return user.click(await screen.findByRole('button', { name: 'Sign out' }));
}

describe('application routes', () => {
  it('redirects anonymous users and establishes a local management session', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      getSession: vi.fn().mockResolvedValue(null),
      login: vi.fn().mockResolvedValue(testSession),
    });
    renderRoutes('/configuration?editor=advanced#deploy', client);
    await user.type(await screen.findByLabelText('Management token'), 'local-token');
    await user.click(screen.getByRole('button', { name: 'Open panel' }));
    expect(client.login).toHaveBeenCalledWith('local-token', expect.any(AbortSignal));
    await waitFor(
      () =>
        expect(screen.getByLabelText('Current route')).toHaveTextContent(
          '/configuration?editor=advanced#deploy',
        ),
      { timeout: 10_000 },
    );
    expect(
      await screen.findByRole('heading', { name: 'Configuration' }, { timeout: 10_000 }),
    ).toBeInTheDocument();
    expect(await screen.findByRole('tab', { name: 'Advanced JSON' })).toBeInTheDocument();
  });

  it('navigates primary management pages through the shared shell', async () => {
    const user = userEvent.setup();
    const originalScrollIntoView = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      'scrollIntoView',
    );
    const scrollIntoView = vi.fn();
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
      configurable: true,
      value: scrollIntoView,
    });

    try {
      renderRoutes('/');
      const coreVersions = await screen.findByRole(
        'link',
        { name: 'Versions' },
        { timeout: 10_000 },
      );
      scrollIntoView.mockClear();
      await user.click(coreVersions);
      expect(await screen.findByRole('heading', { name: 'Versions' })).toBeInTheDocument();
      expect(await screen.findByRole('tab', { name: 'Installed' })).toBeInTheDocument();
      expect(screen.queryByText('Panel online')).not.toBeInTheDocument();
      expect(screen.getByRole('link', { name: 'Panel settings' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
      expect(screen.queryByRole('button', { name: 'Open account menu' })).not.toBeInTheDocument();
      expect(document.getElementById('main-content')).toHaveFocus();
      expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'auto', block: 'start' });
    } finally {
      if (originalScrollIntoView === undefined) {
        Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView');
      } else {
        Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', originalScrollIntoView);
      }
    }
  });

  it('keeps appearance controls and icon-only sign out in the mobile navigation sheet', async () => {
    const user = userEvent.setup();
    const originalInnerWidth = window.innerWidth;
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 390 });

    try {
      renderRoutes('/');
      await user.click(await screen.findByRole('button', { name: 'Open navigation' }));
      expect(screen.getByRole('button', { name: /Theme:/ })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Open language menu' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
    } finally {
      Object.defineProperty(window, 'innerWidth', {
        configurable: true,
        value: originalInnerWidth,
      });
    }
  });

  it('renders the authenticated not-found page for an unknown route', async () => {
    renderRoutes('/missing');
    expect(
      await screen.findByRole('heading', { name: 'This console area does not exist.' }),
    ).toBeInTheDocument();
  });

  it('shows a retryable service error instead of treating an initial outage as logged out', async () => {
    const user = userEvent.setup();
    const getSession = vi
      .fn()
      .mockRejectedValueOnce(new Error('service unavailable'))
      .mockResolvedValueOnce(testSession);
    const client = createMockApiClient({ getSession });
    renderRoutes('/configuration', client);

    expect(
      await screen.findByRole('heading', { name: 'The panel service could not be reached.' }),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText('Management token')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('button', { name: 'Sign out' })).toBeInTheDocument();
    expect(getSession).toHaveBeenCalledTimes(2);
  });

  it('moves to anonymous when the HTTP client invalidates the session', async () => {
    let invalidate = () => undefined;
    const client = createMockApiClient({
      subscribeSessionInvalidated: vi.fn((listener) => {
        invalidate = listener;
        return () => undefined;
      }),
    });
    renderRoutes('/', client);
    expect(await screen.findByRole('button', { name: 'Sign out' })).toBeInTheDocument();

    act(() => invalidate());
    expect(await screen.findByLabelText('Management token')).toBeInTheDocument();
  });

  it('keeps the authenticated view and reports a failed sign out', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      logout: vi.fn().mockRejectedValue(new Error('network unavailable')),
    });
    renderRoutes('/', client);
    await openSignOut(user);

    expect(
      await screen.findByText('Sign out failed; your current session is still active'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
  });

  it('redirects legacy task links into panel logs without losing the selected ID', async () => {
    renderRoutes('/tasks?task=task_1');
    await waitFor(() =>
      expect(screen.getByLabelText('Current route')).toHaveTextContent(
        '/observability?tab=panel&task=task_1',
      ),
    );
    expect(await screen.findByRole('tab', { name: 'Panel logs', hidden: true })).toHaveAttribute(
      'aria-selected',
      'true',
    );
    expect(await screen.findByRole('dialog')).toBeVisible();
  });
});

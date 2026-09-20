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
    renderRoutes('/');
    const coreVersions = await screen.findByRole(
      'link',
      { name: 'Versions' },
      { timeout: 10_000 },
    );
    const scroller = document.querySelector<HTMLDivElement>('.panel-content-scroll')!;
    scroller.scrollTop = 400;
    await user.click(coreVersions);
    expect(await screen.findByRole('heading', { name: 'Versions' })).toBeInTheDocument();
    expect(await screen.findByRole('tab', { name: 'Installed' })).toBeInTheDocument();
    expect(screen.queryByText('Panel online')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Panel settings' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Open account menu' })).not.toBeInTheDocument();
    expect(document.getElementById('main-content')).toHaveFocus();
    expect(scroller.scrollTop).toBe(0);
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

  it.each(['/missing', '/tasks', '/tasks?task=task_1'])('renders the authenticated not-found page for %s', async path => {
    renderRoutes(path);
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

  it('opens task details directly from panel-log links', async () => {
    const client = createMockApiClient();
    renderRoutes('/observability?tab=panel&task=task_1', client);
    expect(await screen.findByRole('tab', { name: 'Panel logs', hidden: true })).toHaveAttribute(
      'aria-selected',
      'true',
    );
    expect(await screen.findByRole('dialog')).toBeVisible();
    await waitFor(() => expect(client.getTask).toHaveBeenCalledWith('task_1', expect.any(AbortSignal)));
  });
});

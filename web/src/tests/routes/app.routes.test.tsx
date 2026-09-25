import { useLocation } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { act, render, screen, waitFor } from '@testing-library/react';

import { ThemeProvider } from '@/theme';
import { Toaster } from '@/components/ui/toast';
import '@/i18n';
import { AppRoutes } from '@/routes/app.routes';
import { pageLoaders } from '@/routes/page-loaders';
import { appearanceTokens } from '@/theme/appearance';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ApiClientProvider } from '@/api/api-client-context';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { AuthSessionProvider } from '@/stores/auth-session-provider';
import { createMockApiClient, testDashboardContext, testSession } from '@/tests/api/mock-api-client';

function LocationProbe() {
  const location = useLocation();
  return (
    <output aria-label='Current route'>{`${location.pathname}${location.search}${location.hash}`}</output>
  );
}

function renderRoutes(initialEntry: string, client = createMockApiClient()) {
  return render(
    <Toaster>
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
      </ApiClientProvider>
    </Toaster>,
  );
}

async function openSignOut(user: ReturnType<typeof userEvent.setup>) {
  return user.click(await screen.findByRole('button', { name: 'Sign out' }));
}

describe('application routes', () => {
  it('shares a speculative download across sidebar hover and focus without navigating', async () => {
    const user = userEvent.setup();
    renderRoutes('/');
    const link = await screen.findByRole('link', { name: 'Versions' });
    const load = vi.spyOn(pageLoaders, '/cores');
    try {
      await user.hover(link);
      expect(load).toHaveBeenCalledTimes(1);
      expect(screen.getByLabelText('Current route')).toHaveTextContent(/^\/$/);
      act(() => link.focus());
      expect(load).toHaveBeenCalledTimes(1);
      await act(() => vi.dynamicImportSettled());
      expect(screen.getByLabelText('Current route')).toHaveTextContent(/^\/$/);
    } finally {
      load.mockRestore();
    }
  });

  it('uses the same breathing indicator through session checks and panel initialization', async () => {
    let resolveSession!: (value: typeof testSession) => void;
    let resolveContext!: (value: typeof testDashboardContext) => void;
    const session = new Promise<typeof testSession>(resolve => {
      resolveSession = resolve;
    });
    const context = new Promise<typeof testDashboardContext>(resolve => {
      resolveContext = resolve;
    });
    const client = createMockApiClient({
      getSession: vi.fn(() => session),
      getDashboardContext: vi.fn(() => context),
    });
    renderRoutes('/', client);
    const checking = await screen.findByText('Checking panel session…');
    expect(checking.closest('main')).toHaveClass('loading-screen');
    expect(checking.closest('main')).toHaveAttribute('aria-busy', 'true');
    expect(document.querySelector('.loading-screen__mark')).toBeInTheDocument();
    await act(async () => resolveSession(testSession));
    const initializing = await screen.findByText('Reading panel context');
    expect(initializing.closest('main')).toHaveClass('loading-screen');
    expect(document.querySelector('.loading-screen__mark')).toBeInTheDocument();
    expect(document.querySelector('[data-slot="skeleton"]')).not.toBeInTheDocument();
    expect(document.querySelector('[data-slot="card"]')).not.toBeInTheDocument();
    await act(async () => resolveContext(testDashboardContext));
    await screen.findByRole('button', { name: 'Sign out' });
    expect(screen.queryByText('Reading panel context')).not.toBeInTheDocument();
  });

  it('keeps the shared saved appearance when signing out and loading login again', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient();
    const view = await client.getPanelSettings();
    const appearance = { theme: 'dark', color: '#C65B13', radius: 8 } as const;
    vi.mocked(client.getPanelSettings).mockResolvedValue({
      ...view, preferences: { ...view.preferences, appearance },
    });
    const assertTokens = () => {
      for (const [name, value] of Object.entries(appearanceTokens(appearance, true))) {
        expect(document.documentElement.style.getPropertyValue(name)).toBe(value);
      }
      expect(document.documentElement).toHaveClass('dark');
    };
    const panel = renderRoutes('/', client);
    await screen.findByRole('button', { name: 'Sign out' });
    await waitFor(assertTokens);
    await openSignOut(user);
    await screen.findByLabelText('Management token');
    assertTokens();
    panel.unmount();

    const meta = document.createElement('meta');
    meta.name = 'sing-box-panel-appearance';
    meta.content = JSON.stringify(appearance);
    document.head.append(meta);
    window.localStorage.clear();
    try {
      const anonymous = createMockApiClient({ getSession: vi.fn().mockResolvedValue(null) });
      renderRoutes('/login', anonymous);
      await screen.findByLabelText('Management token');
      assertTokens();
      expect(anonymous.getPanelSettings).not.toHaveBeenCalled();
      await user.type(screen.getByLabelText('Management token'), 'local-token');
      vi.mocked(anonymous.getPanelSettings).mockResolvedValue({
        ...view, preferences: { ...view.preferences, appearance },
      });
      await user.click(screen.getByRole('button', { name: 'Open panel' }));
      await screen.findByRole('button', { name: 'Sign out' });
      assertTokens();
    } finally {
      meta.remove();
    }
  });

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
      await screen.findByText('The panel service could not be reached.', { selector: '[data-slot="toast-title"]' }),
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
      await screen.findByText('Sign out failed; your current session is still active', { selector: '[data-slot="toast-title"]' }),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
  });

  it('matches the login form content and toggles token visibility without submitting', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({ getSession: vi.fn().mockResolvedValue(null) });
    renderRoutes('/login', client);
    const input = await screen.findByLabelText('Management token');
    expect(await screen.findByRole('heading', { name: 'Welcome back' })).toBeVisible();
    expect(input).toHaveAttribute('type', 'password');
    await user.type(input, 'preview-token');
    await user.click(screen.getByRole('button', { name: 'Show management token' }));
    expect(input).toHaveAttribute('type', 'text');
    expect(input).toHaveValue('preview-token');
    await user.click(screen.getByRole('button', { name: 'Hide management token' }));
    expect(input).toHaveAttribute('type', 'password');
    expect(client.login).not.toHaveBeenCalled();
  });

  it('opens panel logs without a legacy operation dialog', async () => {
    renderRoutes('/observability?tab=panel&task=legacy');
    expect(await screen.findByRole('tab', { name: 'Panel logs' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});

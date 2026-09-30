import { useLocation } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';

import type { ApiClient } from '@/api/api-client';

import { ThemeProvider } from '@/theme';
import { Toaster } from '@/components/ui/toast';
import { AppRoutes } from '@/routes/app.routes';
import { ApiRequestError } from '@/api/api-client';
import '@/i18n';
import { pageLoaders } from '@/routes/page-loaders';
import { appearanceTokens } from '@/theme/appearance';
import { TooltipProvider } from '@/components/ui/tooltip';
import * as pageLoaderModule from '@/routes/page-loaders';
import { ApiClientProvider } from '@/api/api-client-context';
import { TestRouter as MemoryRouter } from '@/tests/test-router';
import { AuthSessionProvider } from '@/stores/auth-session-provider';
import { createDemoApiClient } from '@/api/demo/create-demo-api-client';
import { createMockApiClient, testDashboardContext, testSession } from '@/tests/api/mock-api-client';

function LocationProbe() {
  const location = useLocation();
  return (
    <output aria-label='Current route'>{`${location.pathname}${location.search}${location.hash}`}</output>
  );
}

function renderRoutes(initialEntry: string, client: ApiClient = createMockApiClient()) {
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

beforeAll(async () => {
  // Import modules without populating the prefetch cache exercised below.
  await Promise.all(Object.values(pageLoaders).map(load => load()));
});

describe('application routes', () => {
  it('requests preloading on sidebar hover and focus without navigating', async () => {
    const user = userEvent.setup();
    renderRoutes('/');
    const link = await screen.findByRole('link', { name: 'Versions' });
    const load = vi.spyOn(pageLoaderModule, 'preloadPage');
    try {
      await user.hover(link);
      expect(load).toHaveBeenLastCalledWith('/cores');
      expect(screen.getByLabelText('Current route')).toHaveTextContent(/^\/$/);
      act(() => link.focus());
      expect(load).toHaveBeenLastCalledWith('/cores');
      await act(() => vi.dynamicImportSettled());
      expect(screen.getByLabelText('Current route')).toHaveTextContent(/^\/$/);
    } finally {
      load.mockRestore();
    }
  });

  it('renders the workspace as soon as the session is ready while context loads independently', async () => {
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
    expect(checking.closest('main')).toHaveAttribute('aria-busy', 'true');
    await act(async () => resolveSession(testSession));
    expect(await screen.findByRole('button', { name: 'Sign out' })).toBeVisible();
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
    await screen.findByLabelText('Password');
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
      await screen.findByLabelText('Password');
      assertTokens();
      expect(anonymous.getPanelSettings).not.toHaveBeenCalled();
      await user.type(screen.getByLabelText('Email'), 'admin@example.com');
      await user.type(screen.getByLabelText('Password'), 'local-token');
      vi.mocked(anonymous.getPanelSettings).mockResolvedValue({
        ...view, preferences: { ...view.preferences, appearance },
      });
      await user.click(screen.getByRole('button', { name: 'Sign in' }));
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
    await user.type(await screen.findByLabelText('Email'), 'admin@example.com');
    await user.type(screen.getByLabelText('Password'), 'local-token');
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(client.login).toHaveBeenCalledWith({ email: 'admin@example.com', password: 'local-token' }, expect.any(AbortSignal));
    await waitFor(
      () =>
        expect(screen.getByLabelText('Current route')).toHaveTextContent(
          '/configuration?editor=advanced#deploy',
        ),
    );
    expect(
      await screen.findByRole('heading', { name: 'Configuration' }),
    ).toBeInTheDocument();
    expect(await screen.findByRole('tab', { name: 'Advanced JSON' })).toBeInTheDocument();
  });

  it('navigates primary management pages through the shared shell', async () => {
    const user = userEvent.setup();
    renderRoutes('/');
    const coreVersions = await screen.findByRole(
      'link',
      { name: 'Versions' },
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
    expect(screen.queryByLabelText('Password')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Try again' }));
    expect(await screen.findByRole('button', { name: 'Sign out' })).toBeInTheDocument();
    expect(getSession).toHaveBeenCalledTimes(2);
  });

  it('moves to anonymous when the HTTP client invalidates the session', async () => {
    const listeners = new Set<() => void>();
    const invalidate = () => listeners.forEach(listener => listener());
    const client = createMockApiClient({
      subscribeSessionInvalidated: vi.fn((listener) => {
        listeners.add(listener);
        return () => {
          listeners.delete(listener);
        };
      }),
    });
    renderRoutes('/', client);
    expect(await screen.findByRole('button', { name: 'Sign out' })).toBeInTheDocument();

    act(() => invalidate());
    expect(await screen.findByLabelText('Password')).toBeInTheDocument();
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

  it('toggles password visibility without submitting or changing the value', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({ getSession: vi.fn().mockResolvedValue(null) });
    renderRoutes('/login', client);
    const input = await screen.findByLabelText('Password');
    expect(input).toHaveAttribute('type', 'password');
    await user.type(input, 'preview-password');
    await user.click(screen.getByRole('button', { name: 'Show password' }));
    expect(input).toHaveAttribute('type', 'text');
    expect(input).toHaveValue('preview-password');
    await user.click(screen.getByRole('button', { name: 'Hide password' }));
    expect(input).toHaveAttribute('type', 'password');
    expect(client.login).not.toHaveBeenCalled();
  });

  it('reports validation through a toast and describes the focused invalid field', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({ getSession: vi.fn().mockResolvedValue(null) });
    renderRoutes('/login', client);
    const email = await screen.findByLabelText('Email');
    const password = screen.getByLabelText('Password');
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(email).toHaveFocus();
    expect(email).toBeInvalid();
    expect(email).toHaveAccessibleDescription('Enter a valid email address.');
    expect(await screen.findByText('Enter a valid email address.', { selector: '[data-slot="toast-title"]' })).toBeVisible();

    await user.type(email, 'admin@example.com');
    expect(email).not.toHaveAttribute('aria-describedby');
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(password).toHaveFocus();
    expect(password).toBeInvalid();
    expect(password).toHaveAccessibleDescription('Enter your password to continue.');
    expect(await screen.findByText('Enter your password to continue.', { selector: '[data-slot="toast-title"]' })).toBeVisible();
    expect(client.login).not.toHaveBeenCalled();
  });

  it('keeps submitted credentials after a failed login and restores controls for retry', async () => {
    const user = userEvent.setup();
    let rejectLogin!: (reason: Error) => void;
    const client = createMockApiClient({
      getSession: vi.fn().mockResolvedValue(null),
      login: vi.fn(() => new Promise<typeof testSession>((_resolve, reject) => {
        rejectLogin = reject;
      })),
    });
    renderRoutes('/login', client);
    const email = await screen.findByLabelText('Email');
    const password = screen.getByLabelText('Password');
    await user.type(email, 'admin@example.com');
    await user.type(password, 'incorrect-password{Enter}');
    const submit = screen.getByRole('button', { name: /Signing in/ });
    expect(submit).toBeDisabled();
    expect(password).toBeDisabled();
    await user.click(submit);
    expect(client.login).toHaveBeenCalledTimes(1);

    await act(async () => rejectLogin(new ApiRequestError('Invalid credentials', { status: 401, code: 'unauthorized' })));
    expect(await screen.findByText('The email or password is incorrect.', { selector: '[data-slot="toast-title"]' })).toBeVisible();
    expect(email).toHaveValue('admin@example.com');
    expect(password).toHaveValue('incorrect-password');
    expect(password).toHaveAccessibleDescription('The email or password is incorrect.');
    expect(password).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeEnabled();
  });

  it('opens panel logs without a legacy operation dialog', async () => {
    renderRoutes('/observability?tab=panel&task=legacy');
    expect(await screen.findByRole('tab', { name: 'Panel logs' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
  it.each([
    { change: 'email', email: 'updated@example.com' },
    { change: 'Unicode email', email: '用户@例子.测试' },
    { change: 'password', password: 'updated-password-123' },
    { change: 'both', email: 'updated@example.com', password: 'updated-password-123' },
  ])('signs out only after saving $change and accepts the updated account', async ({ email: nextEmail, password: nextPassword }) => {
    const user = userEvent.setup();
    const client = createDemoApiClient();
    const settings = await client.getPanelSettings();
    await client.savePanelSettings({ revision: settings.revision, preferences: { ...settings.preferences, language: 'en' } });
    renderRoutes('/panel', client);
    const email = nextEmail ?? 'admin@example.com';
    const password = nextPassword ?? 'demo-password-123';
    const emailInput = await screen.findByLabelText('Email');
    if (nextEmail) {
      await user.clear(emailInput);
      await user.type(emailInput, email);
    }
    if (nextPassword) await user.type(screen.getByLabelText('Password', { selector: 'input' }), password);
    expect(await client.getSession()).not.toBeNull();
    await user.click(screen.getByRole('button', { name: 'Save settings' }));
    await screen.findByLabelText('Password');
    await user.type(screen.getByLabelText('Email'), email);
    await user.type(screen.getByLabelText('Password'), password);
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(await screen.findByRole('button', { name: 'Sign out' })).toBeVisible();
    expect((await client.getSession())?.email).toBe(email);
  });
});

import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { act, render, screen, waitFor, within } from '@testing-library/react';

import '@/i18n';
import { AppRoutes } from '@/routes';
import { ThemeProvider } from '@/theme';
import { TooltipProvider } from '@/components/ui/tooltip';
import { ApiClientProvider } from '@/api/api-client-context';
import { AuthSessionProvider } from '@/stores/auth-session-provider';
import { PANEL_STATUS_REFRESH_MS } from '@/components/app-shell/app-shell';
import { createMockApiClient, testArtifacts, testSession, testTask } from '@/tests/api/mock-api-client';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

function LocationProbe() {
  const location = useLocation();
  return <output aria-label='Current route'>{`${location.pathname}${location.search}${location.hash}`}</output>;
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
      () => expect(screen.getByLabelText('Current route')).toHaveTextContent('/configuration?editor=advanced#deploy'),
      { timeout: 10_000 },
    );
    expect(await screen.findByRole(
      'heading',
      { name: 'Configuration' },
      { timeout: 10_000 },
    )).toBeInTheDocument();
    expect(await screen.findByRole('tab', { name: 'Advanced' })).toBeInTheDocument();
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
        { name: 'Core Library' },
        { timeout: 10_000 },
      );
      scrollIntoView.mockClear();
      await user.click(coreVersions);
      expect(await screen.findByRole('heading', { name: 'Core Library' })).toBeInTheDocument();
      expect(await screen.findByText('JSON only')).toBeInTheDocument();
      expect(screen.getByText('Panel online')).toBeInTheDocument();
      expect(screen.getByText('v0.1.0')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
      expect(screen.queryByRole('button', { name: 'Open account menu' })).not.toBeInTheDocument();
      expect(document.getElementById('main-content')).toHaveFocus();
      expect(scrollIntoView).toHaveBeenCalledWith({ behavior: 'auto', block: 'start' });
    } finally {
      if (originalScrollIntoView === undefined) {
        Reflect.deleteProperty(HTMLElement.prototype, 'scrollIntoView');
      } else {
        Object.defineProperty(
          HTMLElement.prototype,
          'scrollIntoView',
          originalScrollIntoView,
        );
      }
    }
  });

  it('keeps panel evidence and icon-only sign out in the mobile navigation sheet', async () => {
    const user = userEvent.setup();
    const originalInnerWidth = window.innerWidth;
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: 390 });

    try {
      renderRoutes('/');
      await user.click(await screen.findByRole('button', { name: 'Open navigation' }));
      expect(await screen.findByText('Panel online')).toBeInTheDocument();
      expect(await screen.findByText('v0.1.0')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
    } finally {
      Object.defineProperty(window, 'innerWidth', {
        configurable: true,
        value: originalInnerWidth,
      });
    }
  });

  it('does not present unavailable panel evidence as online', async () => {
    const client = createMockApiClient({
      getSystemStatus: vi.fn().mockRejectedValue(new Error('status unavailable')),
    });
    renderRoutes('/tasks', client);

    const status = await screen.findByLabelText('Panel status unavailable');
    expect(within(status).getByText('Panel status unavailable').closest('[data-slot="badge"]'))
      .toHaveAttribute('data-variant', 'warning');
    expect(within(status).getByText('—')).toBeInTheDocument();
  });

  it('downgrades online panel evidence when a scheduled refresh fails', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const getSystemStatus = vi.fn()
      .mockResolvedValueOnce({
        panel_version: '0.1.0',
        canonical_revision: 42,
        applied_bundle_id: 'bundle_18',
        running: true,
        running_version: '1.13.19',
        running_artifact: 'core_1',
        configuration_state: 'sing-box-1.13.19@1',
      })
      .mockRejectedValue(new Error('panel disconnected'));

    try {
      renderRoutes('/tasks', createMockApiClient({ getSystemStatus }));
      expect(await screen.findByLabelText('Panel online')).toBeInTheDocument();
      expect(getSystemStatus).toHaveBeenCalledTimes(1);

      await act(async () => vi.advanceTimersByTimeAsync(PANEL_STATUS_REFRESH_MS));
      const unavailable = await screen.findByLabelText('Panel status unavailable');
      expect(within(unavailable).getByText('—')).toBeInTheDocument();
      expect(getSystemStatus).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });

  it('refreshes panel evidence on visibility and clears scheduled work on cleanup', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const getSystemStatus = vi.fn()
      .mockResolvedValueOnce({
        panel_version: '0.1.0',
        canonical_revision: 42,
        applied_bundle_id: 'bundle_18',
        running: true,
        running_version: '1.13.19',
        running_artifact: 'core_1',
        configuration_state: 'sing-box-1.13.19@1',
      })
      .mockRejectedValue(new Error('panel disconnected'));

    try {
      const view = renderRoutes('/tasks', createMockApiClient({ getSystemStatus }));
      expect(await screen.findByLabelText('Panel online')).toBeInTheDocument();

      await act(async () => document.dispatchEvent(new Event('visibilitychange')));
      expect(await screen.findByLabelText('Panel status unavailable'))
        .toBeInTheDocument();
      expect(getSystemStatus).toHaveBeenCalledTimes(2);

      view.unmount();
      await act(async () => vi.advanceTimersByTimeAsync(PANEL_STATUS_REFRESH_MS * 3));
      expect(getSystemStatus).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });

  it('renders the authenticated not-found page for an unknown route', async () => {
    renderRoutes('/missing');
    expect(await screen.findByRole('heading', { name: 'This console area does not exist.' })).toBeInTheDocument();
  });

  it('shows a retryable service error instead of treating an initial outage as logged out', async () => {
    const user = userEvent.setup();
    const getSession = vi.fn()
      .mockRejectedValueOnce(new Error('service unavailable'))
      .mockResolvedValueOnce(testSession);
    const client = createMockApiClient({ getSession });
    renderRoutes('/configuration', client);

    expect(await screen.findByRole('heading', { name: 'The panel service could not be reached.' })).toBeInTheDocument();
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
    const client = createMockApiClient({ logout: vi.fn().mockRejectedValue(new Error('network unavailable')) });
    renderRoutes('/', client);
    await openSignOut(user);

    expect(await screen.findByText('Sign out failed; your current session is still active')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
  });

  it('uploads an amd64 private core with the editable variant', async () => {
    const user = userEvent.setup();
    const importCoreArchive = vi.fn().mockResolvedValue({ ...testTask, id: 'task_core_import' });
    const client = createMockApiClient({ importCoreArchive });
    renderRoutes('/cores', client);
    const archive = new File(['archive'], 'sing-box.tar.gz', { type: 'application/gzip' });

    await user.click(await screen.findByRole('button', { name: 'Import archive' }));
    const importDialog = await screen.findByRole('dialog');
    const importForm = within(importDialog);
    await user.upload(importForm.getByLabelText('Archive'), archive);
    await user.selectOptions(importForm.getByLabelText('Architecture'), 'amd64');
    await user.clear(importForm.getByLabelText('Variant'));
    await user.type(importForm.getByLabelText('Variant'), 'with_quic');
    await user.click(importForm.getByRole('button', { name: 'Upload and verify' }));

    expect(importCoreArchive).toHaveBeenCalledWith({
      archive,
      sourceDescription: 'browser upload',
      exactVersion: '1.13.19',
      architecture: 'amd64',
      variant: 'with_quic',
    });
    expect(await screen.findByText(/task_core_import/)).toBeInTheDocument();
  });

  it('confirms artifact restrictions safely and tracks concurrent row actions independently', async () => {
    const user = userEvent.setup();
    const primary = testArtifacts.items[0];
    const secondary = {
      ...primary,
      id: 'core_2',
      arch: 'amd64' as const,
      archive_sha256: 'e'.repeat(64),
      binary_sha256: 'f'.repeat(64),
      binary_path: '/var/lib/sing-box-panel/artifacts/core_2/sing-box',
    };
    const revokeResult = deferred<typeof primary>();
    const removeResult = deferred<void>();
    const revokeCoreArtifact = vi.fn().mockReturnValue(revokeResult.promise);
    const removeCoreArtifact = vi.fn().mockReturnValue(removeResult.promise);
    const getCoreArtifact = vi.fn(async (id: string) => id === secondary.id ? secondary : primary);
    const client = createMockApiClient({
      getCoreArtifact,
      listCoreArtifacts: vi.fn().mockResolvedValue({ items: [primary, secondary] }),
      removeCoreArtifact,
      revokeCoreArtifact,
    });
    renderRoutes('/cores', client);

    const primaryLabel = '1.13.19 arm64/plain (core_1)';
    const secondaryLabel = '1.13.19 amd64/plain (core_2)';
    await user.click(await screen.findByRole('button', { name: `View details for artifact ${primaryLabel}` }));
    await user.click(await screen.findByRole('button', { name: 'Revoke' }));
    expect(revokeCoreArtifact).not.toHaveBeenCalled();
    expect(screen.getByRole('heading', { name: 'Change artifact trust?' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(revokeCoreArtifact).toHaveBeenCalledWith('core_1'));

    await user.click(screen.getByRole('button', { name: 'Close' }));
    await user.click(await screen.findByRole('button', { name: `View details for artifact ${secondaryLabel}` }));
    await user.click(await screen.findByRole('button', { name: 'Remove' }));
    expect(removeCoreArtifact).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Confirm' }));
    await waitFor(() => expect(removeCoreArtifact).toHaveBeenCalledWith('core_2'));

    await act(async () => {
      removeResult.resolve();
      await removeResult.promise;
    });
    expect(screen.getByRole('heading', { name: 'Core Library' })).toBeInTheDocument();

    await act(async () => {
      revokeResult.resolve({ ...primary, verification_state: 'revoked' });
      await revokeResult.promise;
    });
    await waitFor(() => expect(revokeCoreArtifact).toHaveBeenCalledTimes(1));
  });

  it('appends the next durable task page with its paired cursor', async () => {
    const user = userEvent.setup();
    const newer = { ...testTask, id: 'task-new', created_at: '2026-08-29T08:00:01Z' };
    const older = { ...testTask, id: 'task-old', created_at: '2026-08-29T08:00:00Z' };
    const listTasks = vi.fn()
      .mockResolvedValueOnce({
        items: [newer],
        next: { created_at: newer.created_at, id: newer.id },
      })
      .mockResolvedValueOnce({ items: [older] });
    const client = createMockApiClient({ listTasks });
    renderRoutes('/tasks', client);

    expect(await screen.findByText('task-new')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Load older tasks' }));
    expect(await screen.findByText('task-old')).toBeInTheDocument();
    expect(listTasks).toHaveBeenLastCalledWith(expect.objectContaining({
      beforeID: 'task-new',
      beforeTime: newer.created_at,
    }), undefined);
  });
});

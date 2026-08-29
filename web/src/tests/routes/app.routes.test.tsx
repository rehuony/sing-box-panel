import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { act, render, screen } from '@testing-library/react';

import { AppRoutes } from '@/routes';
import { ApiClientProvider } from '@/api/api-client-context';
import { AuthSessionProvider } from '@/stores/auth-session-provider';
import { createMockApiClient, testSession, testTask } from '@/tests/api/mock-api-client';

function renderRoutes(initialEntry: string, client = createMockApiClient()) {
  return render(
    <ApiClientProvider client={client}>
      <AuthSessionProvider>
        <MemoryRouter initialEntries={[initialEntry]}><AppRoutes /></MemoryRouter>
      </AuthSessionProvider>
    </ApiClientProvider>,
  );
}

describe('application routes', () => {
  it('redirects anonymous users and establishes a local management session', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({
      getSession: vi.fn().mockResolvedValue(null),
      login: vi.fn().mockResolvedValue(testSession),
    });
    renderRoutes('/configuration', client);
    await user.type(await screen.findByLabelText('Management token'), 'local-token');
    await user.click(screen.getByRole('button', { name: 'Open console' }));
    expect(client.login).toHaveBeenCalledWith('local-token', expect.any(AbortSignal));
    await user.click(await screen.findByRole('link', { name: 'Configuration' }));
    expect(await screen.findByRole('heading', { name: 'Manage one configuration across versions.' })).toBeInTheDocument();
  });

  it('navigates primary management pages through the shared shell', async () => {
    const user = userEvent.setup();
    renderRoutes('/');
    await user.click(await screen.findByRole('link', { name: 'Core versions' }));
    expect(await screen.findByRole('heading', { name: 'Follow the version rail.' })).toBeInTheDocument();
    expect(screen.getAllByText('sing-box-1.13.19@1')).not.toHaveLength(0);
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
    expect(await screen.findByRole('heading', { name: 'Control the exact bytes that run.' })).toBeInTheDocument();
  });

  it('keeps the authenticated view and reports a failed sign out', async () => {
    const user = userEvent.setup();
    const client = createMockApiClient({ logout: vi.fn().mockRejectedValue(new Error('network unavailable')) });
    renderRoutes('/', client);
    await user.click(await screen.findByRole('button', { name: 'Sign out' }));

    expect(await screen.findByText('Sign out failed; your current session is still active')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign out' })).toBeInTheDocument();
  });

  it('uploads an amd64 private core with the editable variant', async () => {
    const user = userEvent.setup();
    const importCoreArchive = vi.fn().mockResolvedValue({ ...testTask, id: 'task_core_import' });
    const client = createMockApiClient({ importCoreArchive });
    renderRoutes('/cores', client);
    const archive = new File(['archive'], 'sing-box.tar.gz', { type: 'application/gzip' });

    await user.upload(await screen.findByLabelText('Archive'), archive);
    await user.selectOptions(screen.getByLabelText('Architecture'), 'amd64');
    await user.clear(screen.getByLabelText('Variant'));
    await user.type(screen.getByLabelText('Variant'), 'with_quic');
    await user.click(screen.getByRole('button', { name: 'Upload core archive' }));

    expect(importCoreArchive).toHaveBeenCalledWith({
      archive,
      sourceDescription: 'browser upload',
      exactVersion: '1.13.19',
      architecture: 'amd64',
      variant: 'with_quic',
    });
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

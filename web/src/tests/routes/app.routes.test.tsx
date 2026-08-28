import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { render, screen } from '@testing-library/react';

import { AppRoutes } from '@/routes';
import { ApiClientProvider } from '@/api/api-client-context';
import { AuthSessionProvider } from '@/stores/auth-session-provider';
import { createMockApiClient, testSession } from '@/tests/api/mock-api-client';

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
});

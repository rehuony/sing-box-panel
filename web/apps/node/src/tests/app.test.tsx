import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';

import type { ConsoleStatus } from '@/api/generated';

import { App } from '@/app';

const unenrolled: ConsoleStatus = {
  connection: { since: '2026-11-05T08:00:00Z', state: 'unenrolled' },
  enrollment: null,
  node: { arch: 'amd64', hostname: 'edge-tokyo', os: 'linux', protocol: '1.0', version: 'v1.2.3' },
};

function respond(status: number, body?: unknown): Response {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    headers: { 'Content-Type': status >= 400 ? 'application/problem+json' : 'application/json' },
    status,
  });
}

describe('node console', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('signs in with the console password and shows the node\'s link with its panel', async () => {
    const requests: string[] = [];
    vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET';
      requests.push(`${method} ${input}`);
      if (input === '/api/v1/auth/session' && method === 'GET') return respond(401, { code: 'authentication_required', status: 401 });
      if (input === '/api/v1/auth/session' && method === 'POST') {
        const { password } = JSON.parse(String(init?.body)) as { password: string };
        return password === 'console-password-one'
          ? respond(200, { csrfToken: 'csrf', expiresAt: '2026-11-05T20:00:00Z' })
          : respond(401, { code: 'invalid_credentials', status: 401 });
      }
      if (input === '/api/v1/status') return respond(200, unenrolled);
      if (input === '/api/v1/auth/session' && method === 'DELETE') return respond(204);
      return respond(404, { code: 'operation_not_found', status: 404 });
    }));
    const user = userEvent.setup();
    render(<App />);

    const password = await screen.findByLabelText('Console password');
    await user.type(password, 'wrong-password');
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('The password is incorrect.');

    await user.clear(password);
    await user.type(password, 'console-password-one');
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    expect(await screen.findByText('edge-tokyo')).toBeInTheDocument();
    expect(screen.getAllByText('Not enrolled')).toHaveLength(2);
    expect(screen.getByText('linux/amd64')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Sign out' }));
    await waitFor(() => expect(screen.getByLabelText('Console password')).toBeInTheDocument());
    expect(requests).toContain('DELETE /api/v1/auth/session');
  });

  it('explains a disabled console', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => respond(503, { code: 'console_disabled', status: 503 })));
    render(<App />);
    expect(await screen.findByRole('heading', { name: 'Console disabled' })).toBeInTheDocument();
  });
});

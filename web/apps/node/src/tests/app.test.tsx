import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';

import type { ConsoleStatus } from '@/api/generated';

import { App } from '@/app';

const node = { arch: 'amd64', hostname: 'edge-tokyo', os: 'linux', protocol: '1.0', version: 'v1.2.3' };
const unenrolled: ConsoleStatus = {
  connection: { since: '2026-11-05T08:00:00Z', state: 'unenrolled' },
  enrollment: null,
  node,
};
const enrolled: ConsoleStatus = {
  connection: { since: '2026-11-05T08:00:00Z', state: 'connected' },
  enrollment: { node_id: 'node_7c', panel_endpoint: 'panel.example.com:8443', panel_id: 'panel_1', panel_path: '', trust: 'pin' },
  node,
};

function respond(status: number, body?: unknown): Response {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    headers: { 'Content-Type': status >= 400 ? 'application/problem+json' : 'application/json' },
    status,
  });
}

type Handler = (method: string, init?: RequestInit) => Response | undefined;

/** A console that is signed in and serves status, with the given enrollment handlers. */
function signedIn(initial: ConsoleStatus, handlers: Record<string, Handler> = {}) {
  let status = initial;
  const requests: { method: string; path: string; init?: RequestInit }[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string, init?: RequestInit) => {
    const method = init?.method ?? 'GET';
    requests.push({ method, path: input, init });
    const handled = handlers[`${method} ${input}`]?.(method, init);
    if (handled) {
      if (handled.ok && input === '/api/v1/enrollment') status = await handled.clone().json() as ConsoleStatus;
      return handled;
    }
    if (input === '/api/v1/auth/session' && method === 'GET') return respond(200, { csrfToken: 'csrf', expiresAt: '2026-11-05T20:00:00Z' });
    if (input === '/api/v1/status') return respond(200, status);
    return respond(404, { code: 'operation_not_found', status: 404 });
  }));
  return requests;
}

describe('node console', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('signs in with the console password and leads an unenrolled node with the enrollment form', async () => {
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
    expect(await screen.findByRole('heading', { name: 'Enroll this node' })).toBeInTheDocument();
    expect(screen.getByLabelText('Enrollment code')).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Connection' })).not.toBeInTheDocument();
    expect(screen.getAllByText('edge-tokyo').length).toBeGreaterThan(0);

    await user.click(screen.getByRole('button', { name: 'Sign out' }));
    await waitFor(() => expect(screen.getByLabelText('Console password')).toBeInTheDocument());
    expect(requests).toContain('DELETE /api/v1/auth/session');
  });

  it('enrolls with a code from the panel and explains a rejected one', async () => {
    let attempts = 0;
    const requests = signedIn(unenrolled, {
      'POST /api/v1/enrollment': () => ++attempts === 1
        ? respond(422, { code: 'enrollment_rejected', status: 422 })
        : respond(200, { ...enrolled, connection: { since: '2026-11-05T08:01:00Z', state: 'connecting' } }),
    });
    const user = userEvent.setup();
    render(<App />);

    const code = await screen.findByLabelText('Enrollment code');
    await user.click(screen.getByRole('button', { name: 'Enroll' }));
    expect(screen.getByRole('alert')).toHaveTextContent('Paste the enrollment code from the panel.');

    await user.type(code, 'used-code');
    await user.click(screen.getByRole('button', { name: 'Enroll' }));
    expect(await screen.findByText(/did not accept this code/)).toBeInTheDocument();

    await user.clear(code);
    await user.type(code, 'fresh-code');
    await user.click(screen.getByRole('button', { name: 'Enroll' }));
    const connection = await screen.findByRole('region', { name: 'Connection' });
    expect(connection).toHaveTextContent('Connecting');
    expect(connection).toHaveTextContent('panel.example.com:8443');
    const enrollment = requests.filter(request => request.path === '/api/v1/enrollment').at(-1)!;
    expect(new Headers(enrollment.init?.headers).get('X-CSRF-Token')).toBe('csrf');
    expect(JSON.parse(String(enrollment.init?.body))).toEqual({ code: 'fresh-code' });
  });

  it('unenrolls only after confirming the panel it leaves', async () => {
    const requests = signedIn(enrolled, {
      'DELETE /api/v1/enrollment': () => respond(200, unenrolled),
    });
    const user = userEvent.setup();
    render(<App />);

    expect(await screen.findByRole('region', { name: 'Connection' })).toHaveTextContent('Connected');
    await user.click(screen.getByRole('button', { name: 'Unenroll' }));
    const dialog = screen.getByRole('alertdialog');
    expect(dialog).toHaveTextContent('Unenroll from panel.example.com:8443?');
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(requests.some(request => request.method === 'DELETE' && request.path === '/api/v1/enrollment')).toBe(false);

    await user.click(screen.getByRole('button', { name: 'Unenroll' }));
    await user.click(within(screen.getByRole('alertdialog')).getByRole('button', { name: 'Unenroll' }));
    expect(await screen.findByRole('heading', { name: 'Enroll this node' })).toBeInTheDocument();
    expect(requests.some(request => request.method === 'DELETE' && request.path === '/api/v1/enrollment')).toBe(true);
  });

  it('explains a disabled console', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => respond(503, { code: 'console_disabled', status: 503 })));
    render(<App />);
    expect(await screen.findByRole('heading', { name: 'Console disabled' })).toBeInTheDocument();
  });
});

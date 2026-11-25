import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';

import type { ConsoleStatus } from '@/api/generated';

import { App } from '@/app';

const node = { arch: 'amd64', hostname: 'edge-tokyo', os: 'linux', protocol: '1.0', public_ip: '203.0.113.7', version: 'v1.2.3' };
const readings: Pick<ConsoleStatus, 'host' | 'runtime' | 'traffic'> = {
  host: {
    cpu_count: 4,
    cpu_percent: 12.5,
    disk_total: 64_000_000_000,
    disk_used: 16_000_000_000,
    load_one: 0.42,
    memory_total: 4_000_000_000,
    memory_used: 1_000_000_000,
    sampled_at: '2026-11-05T08:00:00Z',
  },
  runtime: { enabled_version: '1.12.0', pid: 4242, running_version: '1.12.0', started_at: '2026-11-05T07:00:00Z', state: 'running' },
  traffic: { download_bytes: 0, live: null, upload_bytes: 0 },
};
const unenrolled: ConsoleStatus = {
  connection: { since: '2026-11-05T08:00:00Z', state: 'unenrolled' },
  enrollment: null,
  node,
  ...readings,
};
const enrolled: ConsoleStatus = {
  connection: { since: '2026-11-05T08:00:00Z', state: 'connected' },
  enrollment: { node_id: 'node_7c', panel_id: 'panel_1', panel_url: 'https://panel.example.com:8443' },
  node,
  ...readings,
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

  it('signs in with the console password and opens the terminal on the node\'s status', async () => {
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
    expect(await screen.findByText('The password is incorrect.', { selector: '[data-slot="toast-title"]' })).toBeInTheDocument();
    expect(password).toBeInvalid();

    await user.clear(password);
    await user.type(password, 'console-password-one');
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    const terminal = await screen.findByRole('region', { name: 'edge-tokyo' });
    expect(terminal).toHaveTextContent('Not enrolled');
    expect(terminal).toHaveTextContent('1.12.0');
    expect(terminal).toHaveTextContent('203.0.113.7');
    // A running core that reports no counters has unknown rates, not zero ones.
    expect(terminal).toHaveTextContent('the core reports no counters');
    expect(screen.getByRole('log', { name: 'Terminal output' })).toHaveTextContent('Run enroll with a code from the panel.');

    await user.type(screen.getByRole('combobox', { name: 'Command' }), 'exit{Enter}');
    await waitFor(() => expect(screen.getByLabelText('Console password')).toBeInTheDocument());
    expect(requests).toContain('DELETE /api/v1/auth/session');
  });

  it('enrolls with a code typed at the prompt, backs out on an empty one and explains a rejected one', async () => {
    let attempts = 0;
    const requests = signedIn(unenrolled, {
      'POST /api/v1/enrollment': () => ++attempts === 1
        ? respond(422, { code: 'enrollment_rejected', status: 422 })
        : respond(200, { ...enrolled, connection: { since: '2026-11-05T08:01:00Z', state: 'connecting' } }),
    });
    const user = userEvent.setup();
    render(<App />);
    const log = await screen.findByRole('log', { name: 'Terminal output' });

    // Tab completes the command and keeps the cursor on the line.
    const command = screen.getByRole('combobox', { name: 'Command' });
    await user.type(command, 'en');
    await user.keyboard('{Tab}');
    expect(command).toHaveValue('enroll ');
    expect(command).toHaveFocus();
    await user.keyboard('{Enter}');
    await user.type(screen.getByRole('textbox', { name: 'Enrollment code' }), '{Enter}');
    expect(log).toHaveTextContent('Cancelled.');

    await user.type(screen.getByRole('combobox', { name: 'Command' }), 'enroll{Enter}');
    await user.type(screen.getByRole('textbox', { name: 'Enrollment code' }), 'used-code{Enter}');
    expect(await screen.findByText(/did not accept this code/)).toBeInTheDocument();

    await user.type(await screen.findByRole('combobox', { name: 'Command' }), 'enroll{Enter}');
    await user.type(screen.getByRole('textbox', { name: 'Enrollment code' }), 'fresh-code{Enter}');
    expect(await screen.findByText('Enrolled with https://panel.example.com:8443 as node_7c.')).toBeInTheDocument();
    const terminal = screen.getByRole('region', { name: 'edge-tokyo' });
    expect(terminal).toHaveTextContent('https://panel.example.com:8443');
    expect(terminal).toHaveTextContent('Connecting');
    const enrollment = requests.filter(request => request.path === '/api/v1/enrollment').at(-1)!;
    expect(new Headers(enrollment.init?.headers).get('X-CSRF-Token')).toBe('csrf');
    expect(JSON.parse(String(enrollment.init?.body))).toEqual({ code: 'fresh-code' });
  });

  it('completes a command from the Tab menu and unenrolls only after y confirms the panel it leaves', async () => {
    const requests = signedIn(enrolled, {
      'DELETE /api/v1/enrollment': () => respond(200, unenrolled),
    });
    const user = userEvent.setup();
    render(<App />);
    const unenrolls = () => requests.filter(request => request.method === 'DELETE' && request.path === '/api/v1/enrollment');

    expect(await screen.findByRole('region', { name: 'edge-tokyo' })).toHaveTextContent('Connected');
    const log = screen.getByRole('log', { name: 'Terminal output' });

    // Tab opens the menu, then cycles it; Enter fills the line with the chosen command without running it.
    const command = screen.getByRole('combobox', { name: 'Command' });
    await user.click(command);
    await user.keyboard('{Tab}');
    const menu = screen.getByRole('listbox', { name: 'Completions' });
    expect(within(menu).getAllByRole('option').map(option => option.firstChild?.textContent))
      .toEqual(['re-enroll', 'unenroll', 'status', 'clear', 'help', 'exit']);
    await user.keyboard('{Tab}{Tab}');
    expect(command).toHaveValue('unenroll');
    expect(within(menu).getByRole('option', { selected: true })).toHaveTextContent('unenroll');
    await user.keyboard('{Enter}');
    expect(command).toHaveValue('unenroll ');
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(log).not.toHaveTextContent('Unenroll from');

    await user.keyboard('{Enter}');
    expect(log).toHaveTextContent('Unenroll from https://panel.example.com:8443?');
    await user.type(screen.getByRole('textbox', { name: 'Type y to unenroll' }), 'n{Enter}');
    expect(log).toHaveTextContent('Cancelled.');
    expect(unenrolls()).toHaveLength(0);

    await user.type(screen.getByRole('combobox', { name: 'Command' }), 'unenroll{Enter}');
    await user.type(screen.getByRole('textbox', { name: 'Type y to unenroll' }), 'y{Enter}');
    expect(await screen.findByText('Unenrolled from https://panel.example.com:8443.')).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'edge-tokyo' })).toHaveTextContent('Not enrolled');
    expect(unenrolls()).toHaveLength(1);
  });
});

import type { ConsoleStatus, Problem, Session } from './generated';

/** A console request the node answered with a problem. */
export class ConsoleRequestError extends Error {
  constructor(readonly status: number, readonly code: string, message: string) {
    super(message);
    this.name = 'ConsoleRequestError';
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T | undefined> {
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    credentials: 'same-origin',
    headers: { Accept: 'application/json', ...init.headers },
  });
  if (response.status === 204) return undefined;
  if (!response.ok) {
    let problem: Partial<Problem> = {};
    try {
      problem = await response.json() as Partial<Problem>;
    } catch {
      // A body that is not a problem still reports the status.
    }
    throw new ConsoleRequestError(response.status, problem.code ?? 'request_failed', problem.detail ?? problem.title ?? response.statusText);
  }
  return await response.json() as T;
}

/** Returns the current session, or null when the browser has none. */
export async function getSession(): Promise<Session | null> {
  try {
    return (await request<Session>('/auth/session'))!;
  } catch (error) {
    if (error instanceof ConsoleRequestError && error.status === 401) return null;
    throw error;
  }
}

export async function login(password: string): Promise<Session> {
  return (await request<Session>('/auth/session', {
    body: JSON.stringify({ password }),
    headers: { 'Content-Type': 'application/json' },
    method: 'POST',
  }))!;
}

export async function logout(csrfToken: string): Promise<void> {
  await request('/auth/session', { headers: { 'X-CSRF-Token': csrfToken }, method: 'DELETE' });
}

export async function getStatus(): Promise<ConsoleStatus> {
  return (await request<ConsoleStatus>('/status'))!;
}

/** Redeems an enrollment code from the panel; the node replaces any enrollment it has. */
export async function enroll(code: string, csrfToken: string): Promise<ConsoleStatus> {
  return (await request<ConsoleStatus>('/enrollment', {
    body: JSON.stringify({ code }),
    headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken },
    method: 'POST',
  }))!;
}

/** Removes the node's enrollment and ends its session with the panel. */
export async function unenroll(csrfToken: string): Promise<ConsoleStatus> {
  return (await request<ConsoleStatus>('/enrollment', { headers: { 'X-CSRF-Token': csrfToken }, method: 'DELETE' }))!;
}

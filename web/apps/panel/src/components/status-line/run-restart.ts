import type { TFunction } from 'i18next';
import type { QueryClient } from '@tanstack/react-query';

import { logLine } from '@shared/components/terminal';
import { toast } from '@shared/components/ui/toast-manager';

import type { ApiClient } from '@/api/api-client';

import { usePanelRestart } from '@/stores/panel-restart.store';
import { describeRequestError } from '@/components/error-notice';
import { useCommandSession } from '@/stores/command-session.store';

/** How long the page waits for the restarted panel to answer. */
export const RESTART_TIMEOUT_MS = 60_000;
/** How often it asks while it waits. */
const POLL_MS = 1_000;

export type RestartOutcome
  = | { kind: 'restarted' }
    /** Another restart is still under way. */
    | { kind: 'busy' }
    | { kind: 'failed'; reason: string };

interface RestartRun {
  t: TFunction;
  client: ApiClient;
  /** The cache whose every view reads again once the panel is back. */
  cache: QueryClient;
}

function wait(milliseconds: number) {
  return new Promise<void>(resolve => setTimeout(resolve, milliseconds));
}

/**
 * Restarts the panel and waits until it answers again: the restart request
 * returns the start of the process that stops, and the panel is back once its
 * status reports another one. One restart runs at a time; the status line
 * shows it while it runs. Once the panel is back every view reads again.
 */
export async function restartPanel({ cache, client, t }: RestartRun): Promise<RestartOutcome> {
  const state = usePanelRestart.getState();
  if (state.restarting) return { kind: 'busy' };
  state.setRestarting(true);
  try {
    let stopping: string;
    try {
      stopping = (await client.restartPanel()).started_at;
    } catch (error) {
      return { kind: 'failed', reason: describeRequestError(error) };
    }
    const deadline = Date.now() + RESTART_TIMEOUT_MS;
    while (Date.now() < deadline) {
      await wait(POLL_MS);
      try {
        // A panel that is still stopping can hold the request; ask again rather than wait for it.
        const status = await client.getSystemStatus(AbortSignal.timeout(POLL_MS * 3));
        if (status.started_at !== stopping) {
          void cache.invalidateQueries();
          return { kind: 'restarted' };
        }
      } catch {
        // The panel does not answer while it restarts.
      }
    }
    return { kind: 'failed', reason: t('shell.restart.timeout') };
  } finally {
    usePanelRestart.getState().setRestarting(false);
  }
}

/** Says how a restart ended in a toast, for a restart asked for outside the command line or while it is closed. */
export function announceRestart(outcome: RestartOutcome, t: TFunction) {
  if (outcome.kind === 'restarted') toast.add({ title: t('shell.restart.done'), type: 'success' });
  else if (outcome.kind === 'failed') toast.add({ title: outcome.reason, type: 'error' });
}

/**
 * The command line's `reboot` once confirmed: a pending line while the panel
 * restarts, then whether it came back. A toast carries the outcome when the
 * line is closed by then; signing out stops it printing into the next session.
 */
export async function runReboot(run: RestartRun): Promise<void> {
  const session = useCommandSession.getState();
  const { generation } = session;
  const header = logLine('muted', run.t('command.reboot.running'), { pending: true });
  session.print(header);
  const outcome = await restartPanel(run);
  const current = useCommandSession.getState();
  if (current.generation !== generation) return;
  current.settle(header.id);
  if (outcome.kind === 'restarted') current.print(logLine('success', run.t('command.reboot.done')));
  else current.print(logLine('danger', outcome.kind === 'busy' ? run.t('command.reboot.busy') : outcome.reason));
  if (!current.open) announceRestart(outcome, run.t);
}

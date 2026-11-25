import type { TFunction } from 'i18next';
import type { QueryClient } from '@tanstack/react-query';
import type { LogLine } from '@shared/components/terminal';

import { logLine } from '@shared/components/terminal';
import { toast } from '@shared/components/ui/toast-manager';

import type { NodeStatus } from '@/utils/fleet-summary';
import type { ApiClient, FleetCommand, FleetCommandSummary } from '@/api/api-client';

import { invalidateFleetCommand } from '@/api/query-client';
import { useCommandSession } from '@/stores/command-session.store';
import { describeProblem, describeRequestError } from '@/components/error-notice';

export type BatchAction = FleetCommand['action'];

/** The most node names a confirmation lists before it counts the rest. */
const NAMED = 5;

/** The nodes a batch acts on, as the confirmation names them: up to five, then how many more. */
export function batchNames(nodes: readonly NodeStatus[], t: TFunction): string {
  const names = nodes.slice(0, NAMED).map(item => item.node.name).join(', ');
  return nodes.length > NAMED ? `${names} ${t('command.batch.more', { count: nodes.length - NAMED })}` : names;
}

/** What a batch interrupts, which its confirmation says; starting interrupts nothing. */
export function batchWarning(action: BatchAction, t: TFunction): string | null {
  return action === 'start' ? null : t(`command.batch.interrupts.${action}`);
}

/** Whether a batch asks first: every interrupting action, and any action on more than one node. */
export function batchNeedsConfirmation(action: BatchAction, count: number): boolean {
  return action !== 'start' || count > 1;
}

interface BatchRun {
  t: TFunction;
  client: ApiClient;
  /** The cache whose views of the fleet and its nodes the batch can change. */
  cache: QueryClient;
  action: BatchAction;
  /** The version install_core installs. */
  exactVersion?: string;
  nodes: readonly NodeStatus[];
}

/**
 * Runs one action on the nodes as the command line's next result: each
 * node's outcome as it arrives, then the summary. It is not tied to any view,
 * so leaving the command line or the window never cancels it; when its
 * result is out of sight at the end, the line closed or the result replaced
 * by a later command's, a toast carries the summary, or says that the batch
 * stopped reporting before it. One batch runs at a time, and signing out
 * stops following it: the panel still finishes it, but its lines never reach
 * the next session. Once it ends, the views of what it can change read again.
 */
export async function runBatch({ action, cache, client, exactVersion, nodes, t }: BatchRun): Promise<void> {
  const session = useCommandSession.getState();
  if (session.running) {
    session.begin(logLine('danger', t('command.batch.busy')));
    return;
  }
  const { generation } = session;
  const current = () => useCommandSession.getState().generation === generation;
  // The session owns the reader: signing out stops reading at once, as the
  // panel keeps running a confirmed batch whether or not anyone reads it.
  const reading = new AbortController();
  const unsubscribe = useCommandSession.subscribe((state) => {
    if (state.generation !== generation) reading.abort();
  });
  const label = t(`command.batch.action.${action}`);
  const names = new Map(nodes.map(item => [item.node.id, item.node.name]));
  const header = logLine('muted', t('command.batch.running', { action: label, count: nodes.length }), { pending: true });
  session.setRunning(true);
  const shown = session.begin(header);
  session.reserve(nodes.length);
  const print = (line: LogLine) => session.print(shown, line);
  let summary: FleetCommandSummary | null = null;
  let failure: string | null = null;
  const nodeIDs = nodes.map(item => item.node.id);
  try {
    const command: FleetCommand = {
      action,
      node_ids: nodeIDs,
      ...(exactVersion ? { exact_version: exactVersion } : {}),
    };
    for await (const event of client.runFleetCommand(command, reading.signal)) {
      if (!current()) break;
      if ('done' in event) {
        summary = event.done;
        continue;
      }
      const { result } = event;
      const term = names.get(result.node_id) ?? result.node_id;
      if (result.ok) print(logLine('success', t('command.batch.ok'), { term }));
      else if (result.skipped) print(logLine('muted', t('command.batch.skipped'), { term }));
      else print(logLine('danger', t('command.batch.failed', { reason: describeProblem(result.code, result.title ?? result.code ?? '') }), { term }));
    }
  } catch (error) {
    failure = describeRequestError(error);
  } finally {
    unsubscribe();
    if (current()) {
      session.settle(header.id);
      session.setRunning(false);
      void invalidateFleetCommand(cache, nodeIDs);
    }
  }
  if (!current()) return;
  const end = useCommandSession.getState();
  const closed = !end.open || end.result !== shown;
  if (summary === null) {
    // Without a summary some nodes' outcomes are unknown, so the ending is a failure the user must see.
    const reason = failure ?? t('command.batch.incomplete');
    print(logLine('danger', reason));
    if (closed) toast.add({ description: reason, title: t('command.batch.toastIncomplete', { action: label }), type: 'error' });
    return;
  }
  if (failure !== null) print(logLine('danger', failure));
  const line = t('command.batch.summary', summary);
  print(logLine(summary.failed > 0 ? 'warning' : 'success', line));
  if (closed) {
    toast.add({
      description: line,
      title: t('command.batch.toast', { action: label }),
      type: summary.failed > 0 ? 'warning' : 'success',
    });
  }
}

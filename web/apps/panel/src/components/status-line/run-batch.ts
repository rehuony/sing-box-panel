import type { TFunction } from 'i18next';

import { logLine } from '@shared/components/terminal';
import { toast } from '@shared/components/ui/toast-manager';

import type { NodeStatus } from '@/utils/fleet-summary';
import type { ApiClient, FleetCommand, FleetCommandSummary } from '@/api/api-client';

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
  action: BatchAction;
  /** The version install_core installs. */
  exactVersion?: string;
  nodes: readonly NodeStatus[];
}

/**
 * Runs one action on the nodes and prints each node's result to the command
 * output as it arrives, then the summary. It is not tied to any view, so
 * leaving the command line or the window never cancels it; when the command
 * line is closed at the end, a toast carries the summary.
 */
export async function runBatch({ action, client, exactVersion, nodes, t }: BatchRun): Promise<void> {
  const session = useCommandSession.getState();
  const label = t(`command.batch.action.${action}`);
  const names = new Map(nodes.map(item => [item.node.id, item.node.name]));
  const header = logLine('muted', t('command.batch.running', { action: label, count: nodes.length }), { pending: true });
  session.setRunning(true);
  session.print(header);
  let summary: FleetCommandSummary | null = null;
  try {
    const command: FleetCommand = {
      action,
      node_ids: nodes.map(item => item.node.id),
      ...(exactVersion ? { exact_version: exactVersion } : {}),
    };
    for await (const event of client.runFleetCommand(command)) {
      if ('done' in event) {
        summary = event.done;
        continue;
      }
      const { result } = event;
      const term = names.get(result.node_id) ?? result.node_id;
      if (result.ok) session.print(logLine('success', t('command.batch.ok'), { term }));
      else if (result.skipped) session.print(logLine('muted', t('command.batch.skipped'), { term }));
      else session.print(logLine('danger', t('command.batch.failed', { reason: describeProblem(result.code, result.title ?? result.code ?? '') }), { term }));
    }
  } catch (error) {
    session.print(logLine('danger', describeRequestError(error)));
  } finally {
    const current = useCommandSession.getState();
    current.settle(header.id);
    current.setRunning(false);
  }
  if (summary === null) return;
  const line = t('command.batch.summary', summary);
  useCommandSession.getState().print(logLine(summary.failed > 0 ? 'warning' : 'success', line));
  if (!useCommandSession.getState().open) {
    toast.add({
      description: line,
      title: t('command.batch.toast', { action: label }),
      type: summary.failed > 0 ? 'warning' : 'success',
    });
  }
}

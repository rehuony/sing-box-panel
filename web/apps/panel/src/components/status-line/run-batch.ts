import type { TFunction } from 'i18next';
import type { QueryClient } from '@tanstack/react-query';

import { toast } from '@shared/components/ui/toast-manager';
import { logLine, nextEntryId } from '@shared/components/terminal';

import type { NodeStatus } from '@/utils/fleet-summary';
import type { BatchReport } from '@/stores/command-session.store';
import type { ApiClient, FleetCommand, FleetCommandEvent } from '@/api/api-client';

import { queries } from '@/api/queries';
import { coreState } from '@/utils/fleet-summary';
import { ApiRequestError } from '@/api/api-client';
import { invalidateFleetCommand } from '@/api/query-client';
import { useCommandSession } from '@/stores/command-session.store';
import { describeProblem, describeRequestError } from '@/components/error-notice';

export type BatchAction = FleetCommand['action'];

/** The most node names a confirmation lists before it counts the rest. */
const NAMED = 5;

/**
 * How long a broken batch stream keeps trying to follow its command again,
 * counted from the last time it heard from the panel: as long as an updated
 * node may take to come back, since the panel may be reached through it.
 */
const RECONNECT_WINDOW_MS = 5 * 60_000;
/** The pause between attempts to follow a broken batch stream again. */
const RECONNECT_DELAY_MS = 2_000;

/** The nodes a batch acts on, as the confirmation names them: up to five, then how many more. */
export function batchNames(nodes: readonly NodeStatus[], t: TFunction): string {
  const names = nodes.slice(0, NAMED).map(item => item.node.name).join(', ');
  return nodes.length > NAMED ? `${names} ${t('command.batch.more', { count: nodes.length - NAMED })}` : names;
}

/** What a batch on count nodes interrupts, which its confirmation says; starting interrupts nothing. */
export function batchWarning(action: BatchAction, count: number, t: TFunction): string | null {
  if (action === 'start') return null;
  // Several nodes update one after another, which their confirmation says.
  return t(action === 'update' && count > 1 ? 'command.batch.interrupts.updateMany' : `command.batch.interrupts.${action}`);
}

/** Whether a batch asks first: every interrupting action, and any action on more than one node. */
export function batchNeedsConfirmation(action: BatchAction, count: number): boolean {
  return action !== 'start' || count > 1;
}

/**
 * Whether an action fits a node as the fleet last reported it. Every action
 * needs the node online; starting needs a core set that is not running,
 * stopping a core that may run, restarting a core set, and updating an
 * update to run. A node it does not fit would only fail or do nothing, and
 * one offline node would stop an update of the nodes after it.
 */
export function batchFits(action: BatchAction, item: NodeStatus): boolean {
  if (item.node.state !== 'online') return false;
  const core = coreState(item);
  const hasCore = Boolean(item.node.runtime?.enabled_core);
  switch (action) {
    case 'start':
      return hasCore && core !== 'running';
    case 'stop':
      return core === 'running' || core === 'unknown';
    case 'restart':
      return hasCore;
    case 'install_core':
      return true;
    case 'update':
      return item.node.update_available;
  }
}

/** The nodes an action fits, in their order, and how many it leaves out. */
export function batchTargets(
  action: BatchAction,
  nodes: readonly NodeStatus[],
): { left: number; targets: NodeStatus[] } {
  const targets = nodes.filter(item => batchFits(action, item));
  return { left: nodes.length - targets.length, targets };
}

/** Whether following a broken stream again may work: a refusal, a forgotten command or an ended session will not. */
function retryable(error: unknown): boolean {
  return !(error instanceof ApiRequestError && error.status >= 400 && error.status < 500);
}

/** Waits before the next attempt, or until the reader stops. */
function pause(signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, RECONNECT_DELAY_MS);
    signal.addEventListener('abort', () => {
      clearTimeout(timer);
      resolve();
    }, { once: true });
  });
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

/** The version a batch brings: the panel's release for an update, the chosen version for a core install. */
function batchVersion({ action, cache, client, exactVersion }: BatchRun): string | null {
  if (action === 'install_core') return exactVersion ?? null;
  if (action !== 'update') return null;
  return cache.getQueryData(queries.systemUpdate(client).queryKey)?.current_version ?? null;
}

/**
 * Runs one action on the nodes as the command line's next result: one live
 * report of every node, waiting, running or done with its outcome, then the
 * summary. It is not tied to any view, so leaving the command line or the
 * window never cancels it. When its stream breaks after the panel named the
 * command, as it does for a browser reaching the panel through a node the
 * batch restarts, the report says so and the batch follows the command again
 * from the outcomes it has, for up to five minutes without hearing from the
 * panel. When its result is out of sight at the end, the line closed or the
 * result replaced by a later command's, a toast carries the summary, or says
 * that the batch stopped reporting before it. One batch runs at a time, and
 * signing out stops following it: the panel still finishes it, but its report
 * never reaches the next session. Once it ends, the views of what it can
 * change read again.
 */
export async function runBatch(run: BatchRun): Promise<void> {
  const { action, cache, client, exactVersion, nodes, t } = run;
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
  let report: BatchReport = {
    action,
    endedAt: null,
    failure: null,
    id: nextEntryId(),
    kind: 'batch',
    lost: false,
    nodes: nodes.map(({ node }) => ({ id: node.id, name: node.name, version: action === 'update' ? node.build_version || null : null })),
    outcomes: [],
    startedAt: Date.now(),
    summary: null,
    version: batchVersion(run),
  };
  session.setRunning(true);
  const shown = session.begin(report);
  const revise = (change: Partial<BatchReport>) => {
    report = { ...report, ...change };
    useCommandSession.getState().revise(shown, report);
  };
  const nodeIDs = nodes.map(item => item.node.id);
  const command: FleetCommand = {
    action,
    node_ids: nodeIDs,
    ...(exactVersion ? { exact_version: exactVersion } : {}),
  };
  // The command's ID once the panel names it, and since when its stream is broken.
  const progress = { commandID: null as string | null, lostSince: null as number | null };
  const handle = (event: FleetCommandEvent) => {
    progress.lostSince = null;
    if (report.lost) revise({ lost: false });
    if ('command' in event) {
      progress.commandID = event.command.id;
      return;
    }
    if ('done' in event) {
      revise({ endedAt: Date.now(), summary: event.done });
      return;
    }
    const { result } = event;
    revise({
      outcomes: [...report.outcomes, {
        durationMs: result.duration_ms ?? null,
        nodeId: result.node_id,
        ok: result.ok,
        reason: result.ok || result.skipped ? null : describeProblem(result.code, result.title ?? result.code ?? ''),
        skipped: result.skipped ?? false,
      }],
    });
  };
  try {
    let lastError: unknown = null;
    while (report.summary === null && current()) {
      try {
        const events = progress.commandID === null
          ? client.runFleetCommand(command, reading.signal)
          : client.followFleetCommand(progress.commandID, report.outcomes.length, reading.signal);
        for await (const event of events) {
          if (!current()) break;
          handle(event);
        }
        // Without the command's ID there is nothing to follow again.
        if (report.summary !== null || !current() || progress.commandID === null) break;
      } catch (error) {
        if (!current()) break;
        // Before the panel names the command, or once it refuses to follow
        // it, following again cannot help.
        if (progress.commandID === null || !retryable(error)) {
          revise({ failure: describeRequestError(error) });
          break;
        }
        lastError = error;
      }
      progress.lostSince ??= Date.now();
      if (Date.now() - progress.lostSince > RECONNECT_WINDOW_MS) {
        if (lastError !== null) revise({ failure: describeRequestError(lastError) });
        break;
      }
      if (!report.lost) revise({ lost: true });
      await pause(reading.signal);
    }
  } finally {
    unsubscribe();
    if (current()) {
      // Without a summary some nodes' outcomes are unknown, so the ending is a failure the user must see.
      if (report.summary === null) revise({ endedAt: Date.now(), failure: report.failure ?? t('command.batch.incomplete'), lost: false });
      session.setRunning(false);
      void invalidateFleetCommand(cache, nodeIDs);
    }
  }
  if (!current()) return;
  const end = useCommandSession.getState();
  if (end.open && end.result === shown) return;
  const { failure, summary } = report;
  if (summary === null) {
    toast.add({ description: failure ?? undefined, title: t('command.batch.toastIncomplete', { action: label }), type: 'error' });
    return;
  }
  toast.add({
    description: t('command.batch.summary', summary),
    title: t('command.batch.toast', { action: label }),
    type: summary.failed > 0 ? 'warning' : 'success',
  });
}

import type { LogLine, TerminalBlock } from '@shared/components/terminal';

import { create } from 'zustand';
import { appendEntries, settleEntry } from '@shared/components/terminal';

import type { NodeStatus } from '@/utils/fleet-summary';
import type { FleetCommand, FleetCommandSummary } from '@/api/api-client';

/** What the command line asks `[y/N]` about before it runs: a batch, or restarting the panel. */
export type PendingConfirmation
  = | { kind: 'batch'; nodes: readonly NodeStatus[]; action: FleetCommand['action'] }
    | { kind: 'reboot' };

/** One node's outcome in a batch report, as the panel reported it. */
export interface BatchOutcome {
  ok: boolean;
  nodeId: string;
  skipped: boolean;
  /** Why it failed, in the reader's language where the problem is known. */
  reason: string | null;
  /** How long the node's command ran on the panel; null where it never ran. */
  durationMs: number | null;
}

/**
 * A batch as one live entry of the command output: its nodes in the order
 * sent, each node's outcome in the order it arrived, then its summary, or
 * why it ended without one. The command output redraws it as it changes.
 */
export interface BatchReport extends TerminalBlock {
  kind: 'batch';
  /** Its stream broke and it is being followed again. */
  lost: boolean;
  startedAt: number;
  endedAt: number | null;
  /** Why it ended without a summary. */
  failure: string | null;
  /** The version an update or a core install brings, when known. */
  version: string | null;
  action: FleetCommand['action'];
  outcomes: readonly BatchOutcome[];
  summary: FleetCommandSummary | null;
  /** The nodes in the order sent, each with the version it ran when the batch began. */
  nodes: readonly { id: string; name: string; version: string | null }[];
}

export type CommandEntry = LogLine | BatchReport;

interface CommandSessionState {
  /** The command box is open over the workspace. */
  open: boolean;
  /** The identity of the result shown. */
  result: number;
  /** A batch is running; it keeps running while the line is closed. */
  running: boolean;
  /** Ends the signed-in session's result, prompt and history. */
  reset: () => void;
  /** Counts sign-outs, so a batch that outlives its session stops printing into the next one. */
  generation: number;
  /** What the last command printed; it stays across closing and opening until the next command replaces it. */
  entries: CommandEntry[];
  settle: (id: number) => void;
  setOpen: (open: boolean) => void;
  /** What the line asks to confirm; the prompt reads `[y/N]` meanwhile. */
  pending: PendingConfirmation | null;
  setRunning: (running: boolean) => void;
  /**
   * Starts the next result with its first entries, replacing the last one
   * whole, and returns its identity: the box shows one command's result.
   */
  begin: (...entries: CommandEntry[]) => number;
  /** Adds lines to a result while it is the one shown; a result the next command replaced takes none. */
  print: (result: number, ...lines: LogLine[]) => void;
  /** Replaces a result's entry of the same identity, such as a batch report that moved on, while it is shown. */
  revise: (result: number, entry: CommandEntry) => void;
  setPending: (pending: PendingConfirmation | null) => void;
}

/** Earlier command lines, kept while the line closes and opens again. */
export const commandHistory: { current: string[] } = { current: [] };

export const useCommandSession = create<CommandSessionState>()((set, get) => ({
  begin: (...entries) => {
    const result = get().result + 1;
    set({ entries: appendEntries([], entries), result });
    return result;
  },
  entries: [],
  generation: 0,
  open: false,
  pending: null,
  print: (result, ...lines) => set(state => state.result === result
    ? { entries: appendEntries<CommandEntry>(state.entries, lines) }
    : state),
  result: 0,
  reset: () => {
    commandHistory.current = [];
    set(state => ({
      entries: [],
      generation: state.generation + 1,
      open: false,
      pending: null,
      result: state.result + 1,
      running: false,
    }));
  },
  revise: (result, entry) => set(state => state.result === result
    ? { entries: state.entries.map(each => each.id === entry.id ? entry : each) }
    : state),
  running: false,
  setOpen: open => set({ open }),
  setPending: pending => set({ pending }),
  setRunning: running => set({ running }),
  settle: id => set(state => ({ entries: settleEntry(state.entries, id) })),
}));

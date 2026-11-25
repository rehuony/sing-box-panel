import type { LogLine } from '@shared/components/terminal';

import { create } from 'zustand';
import { appendEntries, settleEntry } from '@shared/components/terminal';

import type { FleetCommand } from '@/api/api-client';
import type { NodeStatus } from '@/utils/fleet-summary';

/** What the command line asks `[y/N]` about before it runs: a batch, or restarting the panel. */
export type PendingConfirmation
  = | { kind: 'batch'; nodes: readonly NodeStatus[]; action: FleetCommand['action'] }
    | { kind: 'reboot' };

interface CommandSessionState {
  /** Lines kept beyond the terminal's limit, so the largest batch's result stays whole. */
  extra: number;
  /** The command box is open over the workspace. */
  open: boolean;
  /** The identity of the result shown. */
  result: number;
  /** A batch is running; it keeps running while the line is closed. */
  running: boolean;
  /** Ends the signed-in session's result, prompt and history. */
  reset: () => void;
  /** What the last command printed; it stays across closing and opening until the next command replaces it. */
  entries: LogLine[];
  /** Counts sign-outs, so a batch that outlives its session stops printing into the next one. */
  generation: number;
  settle: (id: number) => void;
  /** Keeps room for a batch's one line a node. */
  reserve: (lines: number) => void;
  setOpen: (open: boolean) => void;
  /** What the line asks to confirm; the prompt reads `[y/N]` meanwhile. */
  pending: PendingConfirmation | null;
  /**
   * Starts the next result with its first lines, replacing the last one
   * whole, and returns its identity: the box shows one command's result.
   */
  begin: (...lines: LogLine[]) => number;
  setRunning: (running: boolean) => void;
  /** Adds lines to a result while it is the one shown; a result the next command replaced takes none. */
  print: (result: number, ...lines: LogLine[]) => void;
  setPending: (pending: PendingConfirmation | null) => void;
}

/** Earlier command lines, kept while the line closes and opens again. */
export const commandHistory: { current: string[] } = { current: [] };

export const useCommandSession = create<CommandSessionState>()((set, get) => ({
  begin: (...lines) => {
    const result = get().result + 1;
    set({ entries: appendEntries([], lines), extra: 0, result });
    return result;
  },
  entries: [],
  extra: 0,
  generation: 0,
  open: false,
  pending: null,
  print: (result, ...lines) => set(state => state.result === result
    ? { entries: appendEntries(state.entries, lines, state.extra) }
    : state),
  result: 0,
  reserve: lines => set(state => ({ extra: Math.max(state.extra, lines) })),
  reset: () => {
    commandHistory.current = [];
    set(state => ({
      entries: [],
      extra: 0,
      generation: state.generation + 1,
      open: false,
      pending: null,
      result: state.result + 1,
      running: false,
    }));
  },
  running: false,
  setOpen: open => set({ open }),
  setPending: pending => set({ pending }),
  setRunning: running => set({ running }),
  settle: id => set(state => ({ entries: settleEntry(state.entries, id) })),
}));

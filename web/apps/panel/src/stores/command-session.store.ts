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
  /** Lines kept beyond the terminal's limit, so the largest batch's output stays whole. */
  extra: number;
  /** The status line reads a command instead of showing the scope and windows. */
  open: boolean;
  /** A batch is running; it keeps running while the line is closed. */
  running: boolean;
  clear: () => void;
  /** Ends the signed-in session's output, prompt and history. */
  reset: () => void;
  /** What commands printed; it stays until it is dismissed or cleared, across opening and closing. */
  entries: LogLine[];
  /** Counts sign-outs, so a batch that outlives its session stops printing into the next one. */
  generation: number;
  settle: (id: number) => void;
  /** Keeps room for a batch's one line a node. */
  reserve: (lines: number) => void;
  setOpen: (open: boolean) => void;
  /** What the line asks to confirm; the prompt reads `[y/N]` meanwhile. */
  pending: PendingConfirmation | null;
  print: (...lines: LogLine[]) => void;
  setRunning: (running: boolean) => void;
  setPending: (pending: PendingConfirmation | null) => void;
}

/** Earlier command lines, kept while the line closes and opens again. */
export const commandHistory: { current: string[] } = { current: [] };

export const useCommandSession = create<CommandSessionState>()(set => ({
  clear: () => set({ entries: [], extra: 0 }),
  entries: [],
  extra: 0,
  generation: 0,
  open: false,
  pending: null,
  print: (...lines) => set(state => ({ entries: appendEntries(state.entries, lines, state.extra) })),
  reserve: lines => set(state => ({ extra: Math.max(state.extra, lines) })),
  reset: () => {
    commandHistory.current = [];
    set(state => ({
      entries: [],
      extra: 0,
      generation: state.generation + 1,
      open: false,
      pending: null,
      running: false,
    }));
  },
  running: false,
  setOpen: open => set({ open }),
  setPending: pending => set({ pending }),
  setRunning: running => set({ running }),
  settle: id => set(state => ({ entries: settleEntry(state.entries, id) })),
}));

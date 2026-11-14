import type { LogLine } from '@shared/components/terminal';

import { create } from 'zustand';
import { appendEntries, settleEntry } from '@shared/components/terminal';

import type { FleetCommand } from '@/api/api-client';
import type { NodeStatus } from '@/utils/fleet-summary';

/** A batch the command line asks `[y/N]` about before it runs. */
interface PendingBatch {
  nodes: readonly NodeStatus[];
  action: FleetCommand['action'];
}

interface CommandSessionState {
  /** The status line reads a command instead of showing the scope and windows. */
  open: boolean;
  /** A batch is running; it keeps running while the line is closed. */
  running: boolean;
  clear: () => void;
  /** What commands printed; it stays until it is dismissed or cleared, across opening and closing. */
  entries: LogLine[];
  /** The batch the line asks to confirm; the prompt reads `[y/N]` meanwhile. */
  pending: PendingBatch | null;
  settle: (id: number) => void;
  setOpen: (open: boolean) => void;
  print: (...lines: LogLine[]) => void;
  setRunning: (running: boolean) => void;
  setPending: (pending: PendingBatch | null) => void;
}

export const useCommandSession = create<CommandSessionState>()(set => ({
  clear: () => set({ entries: [] }),
  entries: [],
  open: false,
  pending: null,
  print: (...lines) => set(state => ({ entries: appendEntries(state.entries, lines) })),
  running: false,
  setOpen: open => set({ open }),
  setPending: pending => set({ pending }),
  setRunning: running => set({ running }),
  settle: id => set(state => ({ entries: settleEntry(state.entries, id) })),
}));

/** Earlier command lines, kept while the line closes and opens again. */
export const commandHistory: { current: string[] } = { current: [] };

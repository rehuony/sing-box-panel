import type { ConsoleStatus } from '@/api/generated';

import type { TrafficRates } from './status-format';

export type LineTone = 'danger' | 'muted' | 'plain' | 'success' | 'warning';

/** The prompt an entry was typed at: the shell, the enrollment code or the unenroll confirmation. */
export type PromptKind = 'code' | 'confirm' | 'shell';

/** One line of the terminal's output. */
export interface LogLine {
  /** When the line was written, in milliseconds since the epoch. */
  at: number;
  id: number;
  kind: 'line';
  text: string;
  /** A command name that leads a help line. */
  term?: string;
  tone: LineTone;
  /** The line reports an event, so it leads with its time. */
  stamp?: boolean;
  /** A detail of the line above it, set in from the left. */
  indent?: boolean;
  /** Work the line announced is still running. */
  pending?: boolean;
  /** An entry echoed after the prompt it was typed at. */
  prompt?: PromptKind;
}

/** What one output of the status command shows. */
export interface Readout {
  now: number;
  rates: TrafficRates | null;
  status: ConsoleStatus | null;
}

/** An output of the status command; the newest one follows the node, earlier ones keep what they showed. */
export interface StatusBlock {
  id: number;
  kind: 'status';
  frozen?: Readout;
}

export type BufferEntry = LogLine | StatusBlock;

/** The most entries the terminal keeps; older ones scroll away. */
const MAX_ENTRIES = 200;

let nextId = 0;

export function logLine(tone: LineTone, text: string, extra: Omit<LogLine, 'at' | 'id' | 'kind' | 'text' | 'tone'> = {}): LogLine {
  nextId += 1;
  return { at: Date.now(), id: nextId, kind: 'line', text, tone, ...extra };
}

/** An entry typed at a prompt, as the terminal echoes it. */
export function echo(prompt: PromptKind, text: string): LogLine {
  return logLine('plain', text, { prompt });
}

export function statusBlock(): StatusBlock {
  nextId += 1;
  return { id: nextId, kind: 'status' };
}

/** Appends entries, dropping the oldest past the limit. */
export function appendEntries(entries: readonly BufferEntry[], added: readonly BufferEntry[]): BufferEntry[] {
  return [...entries, ...added].slice(-MAX_ENTRIES);
}

/** Stops every status output from following the node, keeping what each shows now. */
export function freezeStatus(entries: readonly BufferEntry[], readout: Readout): BufferEntry[] {
  return entries.map(entry => entry.kind === 'status' && !entry.frozen ? { ...entry, frozen: readout } : entry);
}

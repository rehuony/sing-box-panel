export type LineTone = 'danger' | 'muted' | 'plain' | 'success' | 'warning';

/**
 * The prompt an entry was typed at: a shell's, with its user, host and path;
 * a code prompt; or a confirmation that only y answers.
 */
export type Prompt
  = | { kind: 'shell'; user: string; host: string; path: string }
    | { kind: 'code' }
    | { kind: 'confirm' };

/** One line of a terminal's output. */
export interface LogLine {
  /** When the line was written, in milliseconds since the epoch. */
  at: number;
  id: number;
  kind: 'line';
  text: string;
  /** A command or target name that leads the line, in its own column. */
  term?: string;
  tone: LineTone;
  /** An entry echoed after the prompt it was typed at. */
  prompt?: Prompt;
  /** The line reports an event, so it leads with its time. */
  stamp?: boolean;
  /** A detail of the line above it, set in from the left. */
  indent?: boolean;
  /** Work the line announced is still running. */
  pending?: boolean;
}

/** An entry a terminal renders itself, such as a readout that follows live data. */
export interface TerminalBlock {
  id: number;
  kind: string;
}

/** The most entries a terminal keeps; older ones scroll away. */
const MAX_ENTRIES = 200;

let lastId = 0;

/** An identity no other entry in this document has. */
export function nextEntryId(): number {
  lastId += 1;
  return lastId;
}

export function logLine(tone: LineTone, text: string, extra: Omit<LogLine, 'at' | 'id' | 'kind' | 'text' | 'tone'> = {}): LogLine {
  return { at: Date.now(), id: nextEntryId(), kind: 'line', text, tone, ...extra };
}

/** An entry typed at a prompt, as the terminal echoes it. */
export function echo(prompt: Prompt, text: string): LogLine {
  return logLine('plain', text, { prompt });
}

export function isLogLine(entry: LogLine | TerminalBlock): entry is LogLine {
  return entry.kind === 'line';
}

/** Appends entries, dropping the oldest past the limit, which `extra` raises for output that must stay whole. */
export function appendEntries<Entry>(entries: readonly Entry[], added: readonly Entry[], extra = 0): Entry[] {
  return [...entries, ...added].slice(-(MAX_ENTRIES + extra));
}

/** Marks a pending line as settled, keeping every other entry. */
export function settleEntry<Entry extends LogLine | TerminalBlock>(entries: readonly Entry[], id: number): Entry[] {
  return entries.map(entry => entry.id === id && isLogLine(entry) ? { ...entry, pending: false } : entry);
}

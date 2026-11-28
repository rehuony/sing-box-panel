export const coreLevels = ['trace', 'debug', 'info', 'warn', 'error', 'fatal', 'panic'] as const;
export type CoreLevel = (typeof coreLevels)[number];
export interface CoreLine {
  /** The line's sequence number since the view started reading, which it keeps while the buffer moves. */
  id: number;
  prefix: string;
  message: string;
  level: CoreLevel | 'unknown';
}

/** The last lines the view keeps, each parsed once. */
export interface CoreText {
  /** The sequence number of the line still being written, which it keeps once complete. */
  next: number;
  /** The text of the line still being written, after the last line break. */
  partial: string;
  /** The level after the last complete line, which a line naming none continues. */
  level: CoreLine['level'];
  /** The kept lines with text, oldest first; the last is the line still being written when it holds text. */
  lines: readonly CoreLine[];
}

export const emptyCoreText: CoreText = { lines: [], partial: '', next: 0, level: 'unknown' };

const keptLines = 2000;

function parseLine(text: string, id: number, level: CoreLine['level']): CoreLine {
  const match = /^(.*?)\b(TRACE|DEBUG|INFO|WARN|ERROR|FATAL|PANIC)\b(.*)$/.exec(text);
  return match
    ? { id, prefix: match[1], message: match[2] + match[3], level: match[2].toLowerCase() as CoreLevel }
    : { id, prefix: '', message: text, level };
}

/**
 * Appends a chunk and keeps the last 2,000 lines, plus the one still being
 * written. Only the chunk and the line it continues are parsed; the lines
 * kept from before stay the same objects. Lines keep their sequence numbers
 * as older ones are dropped; with reset, the chunk replaces the text and its
 * lines take numbers after every line held so far.
 */
export function appendCoreText(previous: CoreText, incoming: string, reset = false): CoreText {
  if (!incoming && !reset) return previous;
  const held = reset ? { ...emptyCoreText, next: previous.next + 1 } : previous;
  const pieces = (held.partial + incoming).split('\n');
  const partial = pieces.pop() ?? '';
  // The line that was still being written is parsed again with the chunk that continues it.
  const lines = held.partial ? held.lines.slice(0, -1) : [...held.lines];
  let { level, next } = held;
  for (const piece of pieces) {
    if (piece) {
      const line = parseLine(piece, next, level);
      lines.push(line);
      level = line.level;
    }
    next += 1;
  }
  let dropped = 0;
  while (dropped < lines.length && lines[dropped]!.id < next - keptLines) dropped += 1;
  const kept = dropped > 0 ? lines.slice(dropped) : lines;
  if (partial) kept.push(parseLine(partial, next, level));
  return { lines: kept, partial, next, level };
}

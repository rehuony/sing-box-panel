export const coreLevels = ['trace', 'debug', 'info', 'warn', 'error', 'fatal', 'panic'] as const;
export type CoreLevel = (typeof coreLevels)[number];
export interface CoreLine {
  /** The line's sequence number since the view started reading, which it keeps while the buffer moves. */
  id: number;
  prefix: string;
  message: string;
  level: CoreLevel | 'unknown';
}

/** The last lines the view keeps, and the sequence number of the first of them. */
export interface CoreText {
  text: string;
  first: number;
}

export const emptyCoreText: CoreText = { text: '', first: 0 };

const keptLines = 2000;

/** Parses the kept text; each line's ID is its sequence number. */
export function parseCoreLines({ first, text }: CoreText): CoreLine[] {
  let level: CoreLine['level'] = 'unknown';
  return text.split('\n').flatMap((line, index) => {
    if (!line) return [];
    const id = first + index;
    const match = /^(.*?)\b(TRACE|DEBUG|INFO|WARN|ERROR|FATAL|PANIC)\b(.*)$/.exec(line);
    if (match) {
      level = match[2].toLowerCase() as CoreLevel;
      return [{ id, prefix: match[1], message: match[2] + match[3], level }];
    }
    return [{ id, prefix: '', message: line, level }];
  });
}

/**
 * Appends a chunk and keeps the last 2,000 lines, plus the one still being
 * written. Lines keep their sequence numbers as older ones are dropped; with
 * reset, the chunk replaces the text and its lines take numbers after every
 * line held so far.
 */
export function appendCoreText(previous: CoreText, incoming: string, reset = false): CoreText {
  const held = reset ? { text: '', first: previous.first + previous.text.split('\n').length } : previous;
  const lines = (held.text + incoming).split('\n');
  const dropped = Math.max(0, lines.length - keptLines - 1);
  return { text: lines.slice(dropped).join('\n'), first: held.first + dropped };
}

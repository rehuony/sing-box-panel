/** One word of a command line: its value without quotes, and where it stands in the line. */
export interface CommandWord {
  end: number;
  start: number;
  value: string;
}

/**
 * Splits a command line into words at whitespace. A double-quoted part keeps
 * its spaces, so a name with spaces is one word; an unclosed quote runs to the
 * end of the line.
 */
export function splitWords(line: string): CommandWord[] {
  const words: CommandWord[] = [];
  let index = 0;
  while (index < line.length) {
    if (/\s/.test(line[index]!)) {
      index += 1;
      continue;
    }
    const start = index;
    let value = '';
    while (index < line.length && !/\s/.test(line[index]!)) {
      if (line[index] === '"') {
        const close = line.indexOf('"', index + 1);
        const end = close < 0 ? line.length : close;
        value += line.slice(index + 1, end);
        index = close < 0 ? line.length : close + 1;
      } else {
        value += line[index];
        index += 1;
      }
    }
    words.push({ end: index, start, value });
  }
  return words;
}

/** A word as a line holds it: quoted when it has whitespace or quotes would otherwise be lost. */
export function quoteWord(value: string): string {
  return /[\s"]/.test(value) || value === '' ? `"${value.replaceAll('"', '')}"` : value;
}

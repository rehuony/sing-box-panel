export const coreLevels = ['trace', 'debug', 'info', 'warn', 'error', 'fatal', 'panic'] as const;
export type CoreLevel = (typeof coreLevels)[number];
export interface CoreLine {
  id: number;
  prefix: string;
  message: string;
  level: CoreLevel | 'unknown';
}

export function parseCoreLines(text: string): CoreLine[] {
  let level: CoreLine['level'] = 'unknown';
  let offset = 0;
  return text
    .split('\n')
    .filter(Boolean)
    .map((line) => {
      const id = offset;
      offset += line.length + 1;
      const match = /^(.*?)\b(TRACE|DEBUG|INFO|WARN|ERROR|FATAL|PANIC)\b(.*)$/.exec(line);
      if (match) {
        level = match[2].toLowerCase() as CoreLevel;
        return { id, prefix: match[1], message: match[2] + match[3], level };
      }
      return { id, prefix: '', message: line, level };
    });
}

export function appendCoreText(previous: string, incoming: string): string {
  return (previous + incoming).split('\n').slice(-2001).join('\n');
}

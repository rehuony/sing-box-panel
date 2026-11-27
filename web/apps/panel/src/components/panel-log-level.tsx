import { Badge } from '@shared/components/ui/badge';

import type { LogLevel } from '@/api/api-client';

export function PanelLogLevel({ level }: { level: LogLevel }) {
  const variant = level === 'error' || level === 'fatal' ? 'destructive' : level === 'warn' ? 'warning' : level === 'info' ? 'success' : 'secondary';
  return <Badge variant={variant}>{level.toUpperCase()}</Badge>;
}

const levelText: Partial<Record<LogLevel, string>> = {
  warn: 'text-[var(--color-warning-text)]',
  error: 'text-[var(--color-danger-text)]',
  fatal: 'text-[var(--color-danger-text)]',
};

/**
 * A log line's level as a word: a routine one in faint text, so the column
 * recedes, and only a warning or an error in a colour.
 */
export function PanelLogLevelText({ level }: { level: LogLevel }) {
  return <span className={levelText[level] ?? 'text-[var(--color-text-faint)]'}>{level.toUpperCase()}</span>;
}

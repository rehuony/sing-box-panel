import type { LogLevel } from '@/api/api-client';

import { Badge } from '@/components/ui/badge';

export function PanelLogLevel({ level }: { level: LogLevel }) {
  const variant = level === 'error' || level === 'fatal' ? 'destructive' : level === 'warn' ? 'warning' : level === 'info' ? 'success' : 'secondary';
  return <Badge variant={variant}>{level.toUpperCase()}</Badge>;
}

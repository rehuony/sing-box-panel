import { Badge } from '@shared/components/ui/badge';

import type { LogLevel } from '@/api/api-client';

export function PanelLogLevel({ level }: { level: LogLevel }) {
  const variant = level === 'error' || level === 'fatal' ? 'destructive' : level === 'warn' ? 'warning' : level === 'info' ? 'success' : 'secondary';
  return <Badge variant={variant}>{level.toUpperCase()}</Badge>;
}

import type { TFunction } from 'i18next';
import type { LogLine } from '@shared/components/terminal';

import { logLine } from '@shared/components/terminal';

import type { ConsoleStatus, RuntimeReading } from '@/api/generated';

interface EventContext {
  t: TFunction;
  /** Formats an instant as a time of day. */
  clock: (value: string) => string;
  /** The last connection error the log printed, so each prints once. */
  lastError: { current: string | undefined };
}

function runtimeKey(runtime: RuntimeReading | null): string {
  return runtime ? `${runtime.state}:${runtime.pid ?? ''}` : '';
}

function runtimeEvent(runtime: RuntimeReading, t: TFunction): LogLine {
  switch (runtime.state) {
    case 'running':
      return logLine('plain', t('terminal.log.coreRunning', { pid: runtime.pid, version: runtime.running_version }), { stamp: true });
    case 'stopped':
      return logLine('muted', t('terminal.log.coreStopped'), { stamp: true });
    default:
      return logLine('warning', t('terminal.log.coreUnknown'), { stamp: true });
  }
}

/**
 * The lines for a change in the node's status: its link with the panel and
 * its core. The first status prints only what its output does not show: why
 * the node is disconnected, or that it awaits enrollment. A node that keeps
 * failing to connect prints each new error once, not every retry.
 */
export function statusEvents(
  previous: ConsoleStatus | null,
  next: ConsoleStatus,
  { clock, lastError, t }: EventContext,
): LogLine[] {
  const lines: LogLine[] = [];
  const { connection } = next;
  const address = next.enrollment ? next.enrollment.panel_url : '';
  const was = previous?.connection.state;

  switch (connection.state) {
    case 'connected':
      lastError.current = undefined;
      if (was !== undefined && was !== 'connected') lines.push(logLine('success', t('terminal.log.connected', { address }), { stamp: true }));
      break;
    case 'connecting':
      if (was === 'unenrolled') lines.push(logLine('muted', t('terminal.log.connecting', { address }), { stamp: true }));
      break;
    case 'disconnected':
      if (was === undefined || was === 'connected' || was === 'unenrolled' || connection.last_error !== lastError.current) {
        lastError.current = connection.last_error;
        lines.push(logLine('warning', t('terminal.log.disconnected', { address }), { stamp: true }));
        if (connection.last_error) lines.push(logLine('danger', connection.last_error, { indent: true }));
        if (connection.retry_at) lines.push(logLine('muted', t('terminal.log.retryAt', { time: clock(connection.retry_at) }), { indent: true }));
      }
      break;
    case 'unenrolled':
      lastError.current = undefined;
      if (was !== 'unenrolled') lines.push(logLine('muted', t('terminal.log.notEnrolled'), { stamp: true }));
      break;
  }

  if (previous && next.runtime && runtimeKey(previous.runtime) !== runtimeKey(next.runtime)) {
    lines.push(runtimeEvent(next.runtime, t));
  }
  return lines;
}

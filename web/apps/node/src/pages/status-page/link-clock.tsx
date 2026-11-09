import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { Connection } from '@/api/generated';

/** How often the clock counts on. */
const TICK_MS = 1_000;

/** A span of time as hh:mm:ss; hours keep counting past a day. */
function formatElapsed(milliseconds: number): string {
  const seconds = Number.isFinite(milliseconds) && milliseconds > 0 ? Math.floor(milliseconds / 1_000) : 0;
  const pad = (value: number) => String(value).padStart(2, '0');
  return `${pad(Math.floor(seconds / 3_600))}:${pad(Math.floor((seconds % 3_600) / 60))}:${pad(seconds % 60)}`;
}

/**
 * How long the node's link has held its state, counting each second while the
 * node answers and standing still while its status is stale.
 */
export function LinkClock({ connection, ticking }: { connection: Connection; ticking: boolean }) {
  const { t } = useTranslation();
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (!ticking) return;
    const timer = window.setInterval(() => setNow(Date.now()), TICK_MS);
    return () => window.clearInterval(timer);
  }, [ticking]);
  const elapsed = formatElapsed(now - Date.parse(connection.since));
  return (
    <span className='tabular-nums' title={t('terminal.since', { duration: elapsed, state: t(`connection.state.${connection.state}`) })}>
      {elapsed}
    </span>
  );
}

import type { CSSProperties, ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Spinner } from '@shared/components/ui/spinner';

import type { BatchReport } from '@/stores/command-session.store';

import { compactVersion } from '@/utils/compact-version';

/**
 * How many nodes a batch runs at once, as the panel's contract states: an
 * update one at a time, every other action at most four.
 */
const CONCURRENCY = { other: 4, update: 1 } as const;

/** The progress meter's cells, as fine as a pane's meters whatever the node count. */
const CELLS = 40;

/** A version as the panes show it: with its `v`, a long development build shortened. */
function versionLabel(version: string): string {
  return compactVersion(`v${version.replace(/^v/i, '')}`);
}

/** A span of time as a clock reads it: `m:ss`, or `h:mm:ss` from an hour. */
function clock(milliseconds: number): string {
  const seconds = Math.max(0, Math.floor(milliseconds / 1_000));
  const pad = (value: number) => String(value).padStart(2, '0');
  const hours = Math.floor(seconds / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  return hours > 0 ? `${hours}:${pad(minutes)}:${pad(seconds % 60)}` : `${minutes}:${pad(seconds % 60)}`;
}

/** The time a running batch counts to, moving each second until it ends. */
function useNow(running: boolean): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!running) return;
    const timer = setInterval(() => setNow(Date.now()), 1_000);
    return () => clearInterval(timer);
  }, [running]);
  return now;
}

/**
 * One row: its mark, the node or what the row counts, what happened, and how
 * long it took. What happened keeps to one line, unless it wraps, as a
 * failure's reason does, which the reader must see whole.
 */
function Row({ children, mark, name, time, wrap = false }: {
  children?: ReactNode;
  mark: ReactNode;
  name: ReactNode;
  time?: string;
  wrap?: boolean;
}) {
  return (
    // The spaces between cells keep its text apart for assistive technology; the grid lays the cells out.
    <div className='contents'>
      <span aria-hidden='true' className='flex items-center'>{mark}</span>
      {' '}
      <span className='truncate'>{name}</span>
      {' '}
      <span className={wrap ? 'min-w-0 py-1 wrap-anywhere' : 'min-w-0 truncate'}>{children}</span>
      {' '}
      <span className='text-end text-[var(--color-text-faint)] tabular-nums'>{time}</span>
    </div>
  );
}

/**
 * A batch in the command output: its action, node count and the version it
 * brings, with its progress and time; a meter that fills with each outcome's
 * colour in its share of the nodes; each finished node in the order it finished,
 * with what it did or why it failed and how long it took; the running nodes;
 * a broken stream while the batch follows its command again; the nodes still
 * waiting on one row; and the summary, or why it ended without one.
 */
export function BatchReportView({ report }: { report: BatchReport }) {
  const { t } = useTranslation();
  const { action, endedAt, failure, lost, nodes, outcomes, startedAt, summary, version } = report;
  const now = useNow(endedAt === null);
  const total = nodes.length;
  const names = new Map(nodes.map(node => [node.id, node]));
  const reported = new Set(outcomes.map(outcome => outcome.nodeId));
  const left = nodes.filter(node => !reported.has(node.id));
  // The panel starts the nodes in the order sent, as slots free, so the first ones without an outcome run.
  const slots = action === 'update' ? CONCURRENCY.update : CONCURRENCY.other;
  const running = endedAt === null ? left.filter(node => nodes.indexOf(node) < outcomes.length + slots) : [];
  const waiting = left.filter(node => !running.includes(node));
  const target = version ? versionLabel(version) : null;
  const versions = (from: string | null) => from && target
    ? (
        <>
          {versionLabel(from)}
          <span className='text-[var(--color-text-faint)]'>{' → '}</span>
          {target}
        </>
      )
    : null;

  const counts = { failed: 0, ok: 0, skipped: 0 };
  for (const outcome of outcomes) {
    if (outcome.ok) counts.ok += 1;
    else if (outcome.skipped) counts.skipped += 1;
    else counts.failed += 1;
  }
  // Each outcome's colour fills whole cells, its edge where its running count rounds to.
  const edge = (count: number) => `${Math.round(count / Math.max(1, total) * CELLS) / CELLS * 100}%`;
  const meter = {
    '--meter-cells': CELLS,
    'background': `linear-gradient(90deg, var(--color-success) 0 ${edge(counts.ok)}, var(--color-danger) 0 ${edge(counts.ok + counts.failed)}, var(--color-text-faint) 0 ${edge(outcomes.length)}, var(--color-meter-track) 0)`,
  } as CSSProperties;

  return (
    <div className='flex flex-col'>
      <p className='flex min-h-7 items-center gap-[1ch]'>
        <span className='min-w-0 truncate'>
          {t('command.batch.heading', { action: t(`command.batch.action.${action}`), count: total })}
          {target
            ? (
                <>
                  <span className='text-[var(--color-text-faint)]'>{' → '}</span>
                  <span className='text-[var(--color-info-text)]'>{target}</span>
                </>
              )
            : null}
        </span>
        {' '}
        <span className='ms-auto flex-none text-muted-foreground tabular-nums'>
          {endedAt === null
            ? `${outcomes.length}/${total} · ${clock(now - startedAt)}`
            : t('command.batch.took', { time: clock(endedAt - startedAt) })}
        </span>
      </p>
      <p aria-hidden='true' className='flex h-5 items-center'>
        <span className='cell-meter' data-cells='' style={meter} />
      </p>
      <div className='grid auto-rows-[minmax(1.75rem,auto)] grid-cols-[2ch_fit-content(24ch)_minmax(0,1fr)_auto] items-center gap-x-[1ch]'>
        {outcomes.map((outcome) => {
          const node = names.get(outcome.nodeId);
          const time = outcome.durationMs === null ? undefined : clock(outcome.durationMs);
          if (outcome.ok) {
            return (
              <Row key={outcome.nodeId} mark={<span className='text-[var(--color-success-text)]'>✓</span>} name={node?.name ?? outcome.nodeId} time={time}>
                <span className='text-muted-foreground'>
                  {(action === 'update' && versions(node?.version ?? null))
                    || t(`command.batch.done.${action}`, { version: target ?? '' })}
                </span>
              </Row>
            );
          }
          if (outcome.skipped) {
            return (
              <Row key={outcome.nodeId} mark={<span className='text-[var(--color-text-faint)]'>–</span>} name={node?.name ?? outcome.nodeId}>
                <span className='text-muted-foreground'>{t('command.batch.skipped')}</span>
              </Row>
            );
          }
          return (
            <Row key={outcome.nodeId} mark={<span className='text-[var(--color-danger-text)]'>✗</span>} name={node?.name ?? outcome.nodeId} time={time} wrap>
              <span className='text-[var(--color-danger-text)]'>{outcome.reason}</span>
            </Row>
          );
        })}
        {running.map(node => (
          <Row key={node.id} mark={<Spinner className='size-3.5' />} name={node.name}>
            <span className='text-muted-foreground'>
              {t(`command.batch.doing.${action}`)}
              {action === 'update' && versions(node.version)
                ? (
                    <>
                      <span className='text-[var(--color-text-faint)]'>{' · '}</span>
                      {versions(node.version)}
                    </>
                  )
                : null}
            </span>
          </Row>
        ))}
        {lost
          ? (
              <Row mark={<span className='text-[var(--color-warning-text)]'>!</span>} name={<span className='text-[var(--color-warning-text)]'>{t('command.batch.lost')}</span>}>
                <span className='text-muted-foreground'>{t('command.batch.lostDetail')}</span>
              </Row>
            )
          : null}
        {waiting.length > 0
          ? (
              <Row
                mark={<span className='text-[var(--color-text-faint)]'>·</span>}
                name={<span className='text-[var(--color-text-faint)]'>{t(endedAt === null ? 'command.batch.waiting' : 'command.batch.unreported', { count: waiting.length })}</span>}
              >
                <span className='text-[var(--color-text-faint)]' title={waiting.map(node => node.name).join(', ')}>
                  {waiting.map(node => node.name).join(', ')}
                </span>
              </Row>
            )
          : null}
      </div>
      {summary
        ? (
            <p className='flex min-h-7 items-center whitespace-pre-wrap'>
              <span>
                {([
                  ['succeeded', summary.succeeded, 'text-[var(--color-success-text)]'],
                  ['failedCount', summary.failed, 'text-[var(--color-danger-text)]'],
                  ['skippedCount', summary.skipped, 'text-muted-foreground'],
                ] as const).map(([key, count, tone], index) => (
                  <span key={key}>
                    {index > 0 ? <span className='text-[var(--color-text-faint)]'>{' · '}</span> : null}
                    <span className={cn(count > 0 ? tone : 'text-[var(--color-text-faint)]')}>{t(`command.batch.${key}`, { count })}</span>
                  </span>
                ))}
              </span>
            </p>
          )
        : null}
      {failure ? <p className='min-h-7 py-1 text-[var(--color-danger-text)]'>{failure}</p> : null}
    </div>
  );
}

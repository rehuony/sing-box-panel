import type { ReactNode } from 'react';

import { cn } from '@shared/lib/utils';
import { Check, Copy } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Fragment, useEffect, useState } from 'react';
import { CellMeter } from '@shared/components/cell-meter';

/** How long a copy action shows that it copied. */
const COPIED_MS = 1_500;

/**
 * Parts of one value, set apart by a faint dot. A part moves to the next line
 * whole, after its dot, and breaks inside only when it is longer than a line.
 */
export function ReadoutParts({ children, className }: { children: ReactNode[]; className?: string }) {
  const parts = children.filter(part => part !== null && part !== undefined && part !== false && part !== '');
  return (
    <span className={cn('min-w-0', className)}>
      {parts.map((part, index) => (
        // The parts of a value keep their order; their position is their identity.
        // eslint-disable-next-line react/no-array-index-key
        <Fragment key={index}>
          {index > 0 ? ' ' : null}
          <span className='inline-block max-w-full wrap-anywhere'>
            {part}
            {index < parts.length - 1 ? <span aria-hidden='true' className='text-muted-foreground'>{' ·'}</span> : null}
          </span>
        </Fragment>
      ))}
    </span>
  );
}

/** An upload and a download value, each after its arrow. */
export function ReadoutUpDown({ download, upload }: { download: string; upload: string }) {
  const { t } = useTranslation();
  return (
    <span className='inline-flex gap-x-3'>
      <span>
        <span aria-hidden='true'>↑ </span>
        <span className='sr-only'>{t('terminal.value.upload')}</span>
        {upload}
      </span>
      <span>
        <span aria-hidden='true'>↓ </span>
        <span className='sr-only'>{t('terminal.value.download')}</span>
        {download}
      </span>
    </span>
  );
}

/** One row of a readout: its label in the label column, its value beside it. */
export function ReadoutRow({ children, label }: { children: ReactNode; label: string }) {
  return (
    <div className='contents'>
      <dt className='text-muted-foreground'>{label}</dt>
      <dd className='flex min-h-5 min-w-0 items-center'>{children}</dd>
    </div>
  );
}

/** A value that cannot be known. */
export function ReadoutUnavailable() {
  const { t } = useTranslation();
  return <span className='text-muted-foreground'>{t('common.unavailable')}</span>;
}

function CopyButton({ label, value }: { label: string; value: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(setCopied, COPIED_MS, false);
    return () => window.clearTimeout(timer);
  }, [copied]);
  return (
    <Button
      aria-label={copied ? t('terminal.copied') : t('terminal.copy', { label })}
      // The button stays a small mark in the line; its target reaches past it.
      className={cn(
        'relative size-5 min-h-5 rounded-[var(--radius-control)] text-muted-foreground after:absolute after:-inset-2.5',
        'hover:bg-[var(--color-hover)] hover:text-foreground',
      )}
      onClick={() => {
        void navigator.clipboard?.writeText(value).then(() => setCopied(true), () => undefined);
      }}
      size='icon-xs'
      variant='ghost'
    >
      {copied ? <Check aria-hidden='true' className='size-3.5 text-[var(--color-success)]' /> : <Copy aria-hidden='true' className='size-3.5' />}
    </Button>
  );
}

/** An identifier with its copy action. */
export function ReadoutCopyable({ label, value }: { label: string; value: string }) {
  return (
    <span className='inline-flex max-w-full min-w-0 items-center gap-1 align-middle'>
      <span className='truncate' title={value}>{value}</span>
      <CopyButton label={label} value={value} />
    </span>
  );
}

/** A share as ten cells along the meter's scale. Its number beside it carries the value, so a phone leaves it out. */
export function ReadoutMeter({ className, percent }: { className?: string; percent: number | null }) {
  if (percent === null) return null;
  return <CellMeter className={cn('max-sm:hidden', className)} percent={percent} />;
}

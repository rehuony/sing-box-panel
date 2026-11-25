import type { CSSProperties, ReactNode } from 'react';

import { useId } from 'react';
import { cn } from '@shared/lib/utils';
import { Field, FieldGroup, FieldLabel } from '@shared/components/ui/field';

import { InfoTooltip } from '@/components/info-tooltip';

/**
 * A card of field rows on the canvas: a title bar holding its title, an
 * optional mark and actions at the end, then rows divided by hairlines on a
 * narrower label column.
 */
export function SettingsCard({ title, mark, actions, footer, danger, children }: {
  title: string;
  mark?: ReactNode;
  actions?: ReactNode;
  /** The action bar of rows that save together. */
  footer?: ReactNode;
  /** Rows of destructive actions, bordered in danger. */
  danger?: boolean;
  children: ReactNode;
}) {
  const id = useId();
  return (
    <section aria-labelledby={id} className={cn('min-w-0 rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]', danger && 'border-[var(--color-danger-border)]')}>
      <CardTitleBar id={id} title={title}>
        {mark}
        {actions}
      </CardTitleBar>
      <FieldGroup className='gap-0 divide-y [--settings-label:9.5rem] *:data-[slot=field]:min-h-14 *:data-[slot=field]:px-4 *:data-[slot=field]:py-2'>
        {children}
      </FieldGroup>
      {footer ? <div className='flex min-h-14 items-center gap-3 border-t px-4 py-2'>{footer}</div> : null}
    </section>
  );
}

function CardTitleBar({ id, title, children }: { id: string; title: string; children?: ReactNode }) {
  return (
    <div className='flex h-11 items-center gap-2 border-b ps-4 pe-2'>
      <h2 className='min-w-0 flex-1 truncate text-sm font-medium' id={id}>{title}</h2>
      {children}
    </div>
  );
}

/**
 * A settings card whose rows have not arrived: its title bar over a region
 * as tall as its rows and, with `footer`, its action bar, holding the loading
 * or error state, so the card keeps its size when the rows replace it. Each
 * row is 3.5rem with a hairline between rows, the action bar 3.5rem over one.
 */
export function SettingsCardState({ title, rows, footer = false, children }: {
  title: string;
  rows: number;
  footer?: boolean;
  children: ReactNode;
}) {
  const id = useId();
  const bars = footer ? rows + 1 : rows;
  return (
    <section aria-labelledby={id} className='min-w-0 rounded-[var(--radius-card)] border bg-[var(--color-surface-translucent)]'>
      <CardTitleBar id={id} title={title} />
      <div
        className='flex h-[calc(var(--bars)*3.5rem+var(--bars)*1px-1px)] flex-col'
        style={{ '--bars': bars } as CSSProperties}
      >
        {children}
      </div>
    </section>
  );
}

/**
 * One field row: the label column, then the control filling the rest;
 * stacked in narrow groups. A changed row marks its start edge in the accent.
 */
export function SettingsField({ id, label, help, invalid, changed, children }: {
  id: string;
  label: string;
  help?: string;
  invalid?: boolean;
  changed?: boolean;
  children: ReactNode;
}) {
  return (
    <Field
      className='relative grid min-h-11 grid-cols-1 items-center gap-1 @lg/field-group:grid-cols-[var(--settings-label,minmax(8rem,14rem))_minmax(0,1fr)] @lg/field-group:gap-4 has-data-[slot=textarea]:items-start data-[changed=true]:before:absolute data-[changed=true]:before:inset-y-3.5 data-[changed=true]:before:-start-px data-[changed=true]:before:w-0.5 data-[changed=true]:before:rounded-full data-[changed=true]:before:bg-primary'
      data-changed={changed || undefined}
      data-invalid={invalid}
    >
      <div className='flex min-h-10 items-center gap-1'>
        <FieldLabel className='font-normal text-[var(--color-text-secondary)] group-data-[invalid=true]/field:text-[var(--color-danger-text)]' htmlFor={id}>
          {label}
        </FieldLabel>
        {help && <InfoTooltip label={label}>{help}</InfoTooltip>}
      </div>
      <div className='min-w-0 *:data-[slot=input]:w-full *:data-[slot=select-trigger]:w-full'>{children}</div>
    </Field>
  );
}

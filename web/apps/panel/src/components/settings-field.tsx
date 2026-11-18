import type { ReactNode } from 'react';

import { Field, FieldGroup, FieldLabel, FieldLegend, FieldSet } from '@shared/components/ui/field';

import { InfoTooltip } from '@/components/info-tooltip';

/** A titled group of field rows that save together. */
export function SettingsGroup({ title, children }: { title: string; children: ReactNode }) {
  return (
    <FieldSet className='min-w-0 gap-3'>
      <FieldLegend className='mb-0 flex w-full items-center gap-4 text-sm font-semibold text-foreground after:h-px after:flex-1 after:bg-border'>
        {title}
      </FieldLegend>
      <FieldGroup>{children}</FieldGroup>
    </FieldSet>
  );
}

/** One field row: the label column, then the control filling the rest; stacked in narrow groups. */
export function SettingsField({ id, label, help, invalid, children }: {
  id: string;
  label: string;
  help?: string;
  invalid?: boolean;
  children: ReactNode;
}) {
  return (
    <Field
      className='grid min-h-11 grid-cols-1 items-center gap-1 @lg/field-group:grid-cols-[minmax(8rem,14rem)_minmax(0,1fr)] @lg/field-group:gap-4 has-data-[slot=textarea]:items-start'
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

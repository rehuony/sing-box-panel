import type { ReactNode } from 'react';

import { InfoTooltip } from '@/components/info-tooltip';
import { Field, FieldLabel } from '@/components/ui/field';

export function SettingsField({ id, label, help, invalid, children }: {
  id: string;
  label: string;
  help?: string;
  invalid?: boolean;
  children: ReactNode;
}) {
  return (
    <Field className='settings-field' orientation='responsive' data-invalid={invalid}>
      <div className='settings-field__label'>
        <FieldLabel htmlFor={id}>{label}</FieldLabel>
        {help && <InfoTooltip label={label}>{help}</InfoTooltip>}
      </div>
      <div className='settings-field__control'>{children}</div>
    </Field>
  );
}

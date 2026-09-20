import type { ReactNode } from 'react';

import { CircleHelp } from 'lucide-react';

import { Field, FieldLabel } from '@/components/ui/field';
import { Popover, PopoverContent, PopoverDescription, PopoverTrigger } from '@/components/ui/popover';

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
        {help && (
          <Popover>
            <PopoverTrigger openOnHover delay={150} className='settings-help' aria-label={label}>
              <CircleHelp aria-hidden='true' size={16} />
            </PopoverTrigger>
            <PopoverContent className='settings-help-content' side='top' initialFocus={false} aria-label={label}>
              <PopoverDescription>{help}</PopoverDescription>
            </PopoverContent>
          </Popover>
        )}
      </div>
      <div className='settings-field__control'>{children}</div>
    </Field>
  );
}

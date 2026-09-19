import type { ReactNode } from 'react';

import { CircleHelp } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Field, FieldLabel } from '@/components/ui/field';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';

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
          <Tooltip>
            <TooltipTrigger render={<Button type='button' variant='ghost' size='icon-sm' aria-label={label} />}>
              <CircleHelp />
            </TooltipTrigger>
            <TooltipContent>{help}</TooltipContent>
          </Tooltip>
        )}
      </div>
      <div className='settings-field__control'>{children}</div>
    </Field>
  );
}

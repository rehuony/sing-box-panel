import type { ReactNode } from 'react';

import { Info } from 'lucide-react';

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
            <TooltipTrigger aria-label={label} render={<Button className='focus-visible:border-transparent focus-visible:ring-0' size='icon-xs' type='button' variant='ghost' />}>
              <Info aria-hidden />
            </TooltipTrigger>
            <TooltipContent>{help}</TooltipContent>
          </Tooltip>
        )}
      </div>
      <div className='settings-field__control'>{children}</div>
    </Field>
  );
}

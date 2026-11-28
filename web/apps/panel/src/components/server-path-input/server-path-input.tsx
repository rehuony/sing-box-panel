import type { ComponentProps } from 'react';

import { useState } from 'react';
import { FolderOpen } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Dialog, DialogContent, DialogTrigger } from '@shared/components/ui/dialog';
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from '@shared/components/ui/input-group';

import type { FilesystemMode } from '@/api/api-client';

import { ServerPathPicker } from './server-path-picker';

export interface ServerPathInputProps extends Omit<ComponentProps<'input'>, 'value' | 'onChange' | 'type'> {
  value: string;
  mode: FilesystemMode;
  onValueChange: (value: string) => void;
}

export function ServerPathInput({ value, mode, onValueChange, disabled, readOnly, ...props }: ServerPathInputProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [initial, setInitial] = useState<string | null>(null);
  return (
    <Dialog open={open && !disabled && !readOnly} onOpenChange={next => {
      if (next && (disabled || readOnly)) return;
      if (next) setInitial(value);
      setOpen(next);
    }} onOpenChangeComplete={next => {
      if (!next) setInitial(null);
    }}>
      <InputGroup className='has-[[data-browse]:focus-visible]:outline-none'>
        <InputGroupInput {...props} type='text' autoComplete='off' spellCheck={false}
          value={value} disabled={disabled} readOnly={readOnly}
          onChange={event => onValueChange(event.currentTarget.value)} />
        <InputGroupAddon align='inline-end'>
          <DialogTrigger render={<InputGroupButton className='border-transparent bg-transparent text-muted-foreground shadow-none outline-none hover:border-transparent hover:bg-transparent focus-visible:shadow-[inset_0_0_0_1px_var(--ring)] focus-visible:outline-none aria-expanded:bg-transparent active:scale-100' data-browse disabled={disabled || readOnly} aria-label={t('filesystem.browseField', { field: props['aria-label'] ?? props.name ?? '' })} />}>
            <FolderOpen data-icon='inline-start' aria-hidden='true' />
            {t('filesystem.browse')}
          </DialogTrigger>
        </InputGroupAddon>
      </InputGroup>
      <DialogContent className='min-h-0 grid-rows-[auto_minmax(0,1fr)] overflow-hidden max-sm:p-5' size='workspace'>
        {initial !== null && (
          <ServerPathPicker initialValue={initial} mode={mode} open={open && !disabled && !readOnly}
            onSelect={path => {
              onValueChange(path);
              setOpen(false);
            }} />
        )}
      </DialogContent>
    </Dialog>
  );
}

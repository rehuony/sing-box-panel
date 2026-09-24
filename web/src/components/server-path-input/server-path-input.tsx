import type { ComponentProps } from 'react';

import { useState } from 'react';
import { FolderOpen } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { FilesystemMode } from '@/api/api-client';

import { Dialog, DialogContent, DialogTrigger } from '@/components/ui/dialog';
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from '@/components/ui/input-group';

import { ServerPathPicker } from './server-path-picker';
import './server-path-input.css';

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
      <InputGroup className='server-path-input'>
        <InputGroupInput {...props} type='text' autoComplete='off' spellCheck={false}
          value={value} disabled={disabled} readOnly={readOnly}
          onChange={event => onValueChange(event.currentTarget.value)} />
        <InputGroupAddon align='inline-end'>
          <DialogTrigger render={<InputGroupButton className='server-path-input__browse' disabled={disabled || readOnly} aria-label={t('filesystem.browseField', { field: props['aria-label'] ?? props.name ?? '' })} />}>
            <FolderOpen data-icon='inline-start' aria-hidden='true' />
            {t('filesystem.browse')}
          </DialogTrigger>
        </InputGroupAddon>
      </InputGroup>
      <DialogContent className='min-h-0 grid-rows-[auto_minmax(0,1fr)] gap-5 overflow-hidden p-5 sm:max-w-2xl sm:p-6 lg:max-w-[60rem]'>
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

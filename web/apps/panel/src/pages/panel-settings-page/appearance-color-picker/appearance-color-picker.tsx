import { useTranslation } from 'react-i18next';
import { useId, useRef, useState } from 'react';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { HexColorPicker, setNonce } from 'react-colorful';
import { Field, FieldLabel } from '@shared/components/ui/field';
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@shared/components/ui/dialog';

import { ErrorNotice } from '@/components/error-notice';

import './appearance-color-picker.css';

export function AppearanceColorPicker({ id, value, onChange }: {
  id: string;
  value: string;
  onChange: (color: string) => void;
}) {
  const { t } = useTranslation();
  const hexId = useId();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(value);
  const [pickerColor, setPickerColor] = useState(value);
  const pendingColorRef = useRef<string | null>(null);
  const valid = /^#[\dA-F]{6}$/i.test(draft);

  function updateColor(color: string) {
    const next = color.toUpperCase();
    setDraft(next);
    if (/^#[\dA-F]{6}$/i.test(next)) setPickerColor(next);
  }

  return (
    <Dialog open={open} onOpenChange={next => {
      if (next) {
        // The picker injects its stylesheet on mount under the panel's CSP.
        setNonce(document.querySelector<HTMLMetaElement>('meta[name="sing-box-panel-style-nonce"]')?.content ?? '');
        setDraft(value);
        setPickerColor(value);
      }
      setOpen(next);
    }} onOpenChangeComplete={next => {
      if (!next && pendingColorRef.current !== null) {
        onChange(pendingColorRef.current);
        pendingColorRef.current = null;
      }
    }}>
      <DialogTrigger aria-label={t('panelSettings.customColor')} render={<Button id={id} type='button' variant='outline' className='shrink-0 gap-2 rounded-[var(--radius-control)] px-3' />}>
        <span className='block size-4 flex-none rounded-full shadow-[inset_0_0_0_1px_var(--border)]' style={{ backgroundColor: value }} aria-hidden='true' />
        {t('panelSettings.customColor')}
      </DialogTrigger>
      <DialogContent className='w-[min(420px,calc(100vw-32px))] gap-5'>
        <DialogHeader><DialogTitle>{t('panelSettings.customColorTitle')}</DialogTitle></DialogHeader>
        <HexColorPicker className='appearance-color-picker h-auto! w-full! gap-5' color={pickerColor} onChange={updateColor} />
        <Field className='grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-4 gap-y-2' data-invalid={!valid}>
          <FieldLabel htmlFor={hexId}>HEX</FieldLabel>
          <Input
            id={hexId}
            aria-label={t('panelSettings.hex')}
            aria-invalid={!valid}
            aria-describedby={!valid ? `${hexId}-error` : undefined}
            autoComplete='off'
            className='font-mono'
            spellCheck={false}
            maxLength={7}
            value={draft}
            onChange={event => updateColor(event.target.value)}
          />
          {!valid && <ErrorNotice id={`${hexId}-error`} error={t('panelSettings.invalidColor')} />}
        </Field>
        <DialogFooter>
          <DialogClose render={<Button type='button' variant='outline' />}>{t('panelSettings.cancel')}</DialogClose>
          <Button type='button' disabled={!valid} onClick={() => {
            pendingColorRef.current = draft;
            setOpen(false);
          }}>
            {t('panelSettings.applyColor')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

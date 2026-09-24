import { useTranslation } from 'react-i18next';
import { useId, useRef, useState } from 'react';
import { HexColorPicker, setNonce } from 'react-colorful';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { ErrorNotice } from '@/components/error-notice';
import { Field, FieldLabel } from '@/components/ui/field';
import { Dialog, DialogClose, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';

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
      <DialogTrigger aria-label={t('panelSettings.customColor')} render={<Button id={id} type='button' variant='outline' className='settings-custom-color' />}>
        <span className='settings-color-swatch' style={{ backgroundColor: value }} aria-hidden='true' />
        {t('panelSettings.customColor')}
      </DialogTrigger>
      <DialogContent className='appearance-color-dialog'>
        <DialogHeader><DialogTitle>{t('panelSettings.customColorTitle')}</DialogTitle></DialogHeader>
        <HexColorPicker className='appearance-color-picker' color={pickerColor} onChange={updateColor} />
        <Field className='appearance-color-hex' data-invalid={!valid}>
          <FieldLabel htmlFor={hexId}>HEX</FieldLabel>
          <Input
            id={hexId}
            aria-label={t('panelSettings.hex')}
            aria-invalid={!valid}
            aria-describedby={!valid ? `${hexId}-error` : undefined}
            autoComplete='off'
            spellCheck={false}
            maxLength={7}
            value={draft}
            onChange={event => updateColor(event.target.value)}
          />
          {!valid && <ErrorNotice id={`${hexId}-error`} error={t('panelSettings.invalidColor')} />}
        </Field>
        <DialogFooter>
          <DialogClose render={<Button type='button' variant='outline' />}>{t('panelSettings.cancel')}</DialogClose>
          <Button type='button' variant='outline' className='appearance-color-apply' disabled={!valid} onClick={() => {
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

import { Copy } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Label } from '@shared/components/ui/label';
import { Button } from '@shared/components/ui/button';
import { formatDateTime } from '@shared/lib/format-time';
import { Textarea } from '@shared/components/ui/textarea';
import { toast } from '@shared/components/ui/toast-manager';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { NodeEnrollment } from '@/api/api-client';

function CopyValue({ id, label, value }: { id: string; label: string; value: string }) {
  const { t } = useTranslation();
  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      toast.add({ title: t('enrollment.copied', { label }), type: 'success' });
    } catch {
      toast.add({ title: t('enrollment.copyFailed'), type: 'error' });
    }
  }
  return (
    <div className='flex min-w-0 flex-col gap-1.5'>
      <div className='flex items-center justify-between gap-2'>
        <Label htmlFor={id}>{label}</Label>
        <Button aria-label={t('enrollment.copy', { label })} onClick={() => void copy()} size='xs' variant='ghost'>
          <Copy aria-hidden='true' data-icon='inline-start' />
          {t('enrollment.copyShort')}
        </Button>
      </div>
      <Textarea
        className='min-h-0 resize-none font-mono text-xs break-all'
        id={id}
        onFocus={event => event.currentTarget.select()}
        readOnly
        rows={3}
        spellCheck={false}
        value={value}
      />
    </div>
  );
}

/**
 * Shows a node's one-time enrollment code. The panel keeps only a hash of
 * the code, so closing the dialog discards it.
 */
export function NodeEnrollmentDialog({ enrollment, onClose, title }: {
  enrollment: NodeEnrollment | null;
  onClose: () => void;
  title: string;
}) {
  const { i18n, t } = useTranslation();
  const locale = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  return (
    <Dialog
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      open={enrollment !== null}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>
            {enrollment === null ? '' : t('enrollment.description', { name: enrollment.node.name })}
          </DialogDescription>
        </DialogHeader>
        {enrollment === null
          ? null
          : (
              <div className='flex min-w-0 flex-col gap-4'>
                <CopyValue id='enrollment-command' label={t('enrollment.command')} value={enrollment.command} />
                <CopyValue id='enrollment-code' label={t('enrollment.code')} value={enrollment.code} />
                <p className='rounded-[var(--radius-control)] border border-[var(--color-warning-border)] bg-[var(--color-warning-soft)] px-3 py-2 text-xs text-[var(--color-warning-text)]'>
                  {t('enrollment.once', { time: formatDateTime(enrollment.expires_at, locale) })}
                </p>
              </div>
            )}
        <DialogFooter>
          <DialogClose render={<Button />}>{t('enrollment.done')}</DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

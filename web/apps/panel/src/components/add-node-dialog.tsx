import type { FormEvent } from 'react';

import { useState } from 'react';
import { Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Label } from '@shared/components/ui/label';
import { useMutation } from '@tanstack/react-query';
import { Button } from '@shared/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { NodeEnrollment } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { describeRequestError } from '@/components/error-notice';
import { NodeEnrollmentDialog } from '@/components/node-enrollment-dialog';

/** Registers a node by name, then shows the one-time code that enrolls it. */
/** Adds a node and shows its enrollment code; in a strip on a phone the action keeps only its icon. */
export function AddNodeDialog({ iconOnPhone = false, onAdded }: { iconOnPhone?: boolean; onAdded: () => void }) {
  const { t } = useTranslation();
  const client = useApiClient();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [missing, setMissing] = useState(false);
  const [enrollment, setEnrollment] = useState<NodeEnrollment | null>(null);
  const create = useMutation({
    mutationFn: (value: string) => client.createNode(value),
    onSuccess: (result) => {
      setOpen(false);
      setEnrollment(result);
      onAdded();
    },
  });
  const trimmed = name.trim();

  function submit(event: FormEvent) {
    event.preventDefault();
    if (create.isPending) return;
    if (trimmed === '') setMissing(true);
    else create.mutate(trimmed);
  }

  return (
    <>
      <Button
        className={iconOnPhone ? 'max-sm:w-(--control-height) max-sm:px-0' : undefined}
        onClick={() => {
          setName('');
          setMissing(false);
          create.reset();
          setOpen(true);
        }}
      >
        <Plus aria-hidden='true' data-icon='inline-start' />
        <span className={iconOnPhone ? 'max-sm:sr-only' : undefined}>{t('fleet.add.action')}</span>
      </Button>
      <Dialog
        onOpenChange={setOpen}
        open={open}
      >
        <DialogContent>
          <form className='flex flex-col gap-6' noValidate onSubmit={submit}>
            <DialogHeader>
              <DialogTitle>{t('fleet.add.title')}</DialogTitle>
              <DialogDescription>{t('fleet.add.description')}</DialogDescription>
            </DialogHeader>
            <div className='flex flex-col gap-1.5'>
              <Label htmlFor='node-name'>{t('fleet.add.name')}</Label>
              <Input
                aria-describedby='node-name-error'
                aria-invalid={missing || create.error ? true : undefined}
                autoComplete='off'
                id='node-name'
                maxLength={64}
                onChange={(event) => {
                  setName(event.target.value);
                  setMissing(false);
                  if (create.error) create.reset();
                }}
                placeholder='edge-tokyo'
                spellCheck={false}
                value={name}
              />
            </div>
            <DialogFooter className='justify-between'>
              <p className='min-h-5 min-w-0 flex-1 truncate text-sm text-[var(--color-danger-text)]' id='node-name-error' role='status'>
                {create.error ? describeRequestError(create.error) : missing ? t('fleet.add.nameRequired') : ''}
              </p>
              <Button onClick={() => setOpen(false)} type='button' variant='outline'>{t('common.cancel')}</Button>
              <Button type='submit'>
                {t(create.isPending ? 'fleet.add.adding' : 'fleet.add.submit')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <NodeEnrollmentDialog enrollment={enrollment} onClose={() => setEnrollment(null)} title={t('enrollment.title')} />
    </>
  );
}

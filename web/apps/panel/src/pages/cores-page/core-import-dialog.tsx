import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Input } from '@shared/components/ui/input';
import { Button } from '@shared/components/ui/button';
import { Spinner } from '@shared/components/ui/spinner';
import { CloudUpload, FileArchive } from 'lucide-react';
import { Field, FieldGroup, FieldLabel } from '@shared/components/ui/field';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@shared/components/ui/dialog';

import type { CoreImportUpload } from '@/api/api-client';

import { ErrorNotice } from '@/components/error-notice';

export function CoreImportDialog({
  architecture,
  onClose,
  onImport,
}: {
  architecture: 'amd64' | 'arm64';
  onClose: () => void;
  onImport: (input: CoreImportUpload) => Promise<boolean>;
}) {
  const { t } = useTranslation();
  const id = useId();
  const [selection, setSelection] = useState<{ archive: File | null; version: string }>({
    archive: null, version: '',
  });
  const { archive, version } = selection;
  const [busy, setBusy] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [fileError, setFileError] = useState<'singleArchive' | 'archiveFormat' | 'unsupportedBuild' | null>(null);
  const valid = archive !== null && !fileError && /^\d+\.\d+\.\d+$/.test(version.trim());

  function selectArchive(files: File[]) {
    if (busy || files.length === 0) return;
    if (files.length !== 1) {
      setFileError('singleArchive');
      return;
    }
    const [file] = files;
    if (!/\.(?:tar\.gz|tgz)$/i.test(file.name)) {
      setFileError('archiveFormat');
      return;
    }
    const platform = /^sing-box-v?\d+\.\d+\.\d+-linux-(.+)\.(?:tar\.gz|tgz)$/i.exec(file.name);
    if (platform && platform[1] !== `${architecture}-musl`) {
      setFileError('unsupportedBuild');
      return;
    }
    const match = /^sing-box-v?(\d+\.\d+\.\d+)(?:-linux-[\w.-]+)?\.(?:tar\.gz|tgz)$/i.exec(file.name);
    setSelection({ archive: file, version: match?.[1] ?? '' });
    setFileError(null);
  }

  return (
    <Dialog open onOpenChange={(open) => !open && !busy && onClose()}>
      <DialogContent showCloseButton={!busy}>
        <DialogHeader className='gap-1.5'>
          <DialogTitle>{t('cores.import.title')}</DialogTitle>
          <DialogDescription>{t('cores.import.description')}</DialogDescription>
        </DialogHeader>
        <form
          className='flex min-w-0 flex-col gap-5'
          aria-busy={busy}
          onSubmit={async (event) => {
            event.preventDefault();
            if (busy || !archive || !valid) return;
            setBusy(true);
            try {
              if (await onImport({
                archive,
                exactVersion: version.trim(),
                sourceDescription: archive.name,
                variant: 'musl',
                architecture,
              })) {
                onClose();
              }
            } finally {
              setBusy(false);
            }
          }}
        >
          <FieldGroup className='gap-4'>
            <Field data-disabled={busy} data-invalid={!!fileError}>
              <FieldLabel htmlFor={`${id}-archive`} className='sr-only'>{t('cores.import.archive')}</FieldLabel>
              <div
                className='relative flex min-h-40 min-w-0 flex-col items-center justify-center gap-3.5 rounded-[var(--radius-control)] border border-dashed bg-muted px-5 py-6 text-foreground transition-[background-color,border-color] duration-150 has-focus-visible:border-primary data-[disabled=true]:opacity-50 data-[dragging=true]:border-primary data-[dragging=true]:bg-accent data-[invalid=true]:border-destructive [&:not([data-disabled=true]):hover]:border-[var(--color-border-strong)] [&>svg]:pointer-events-none [&>svg]:size-8 [&>svg]:stroke-[1.5] [&>svg]:text-muted-foreground'
                data-dragging={dragging}
                data-disabled={busy}
                data-invalid={!!fileError}
                onDragOver={(event) => {
                  event.preventDefault();
                  const acceptsFiles = event.dataTransfer.types.includes('Files') && !busy;
                  event.dataTransfer.dropEffect = acceptsFiles ? 'copy' : 'none';
                  setDragging(acceptsFiles);
                }}
                onDragLeave={(event) => {
                  if (!(event.relatedTarget instanceof Node && event.currentTarget.contains(event.relatedTarget))) {
                    setDragging(false);
                  }
                }}
                onDrop={(event) => {
                  event.preventDefault();
                  setDragging(false);
                  selectArchive(Array.from(event.dataTransfer.files));
                }}
              >
                {archive ? <FileArchive aria-hidden='true' /> : <CloudUpload aria-hidden='true' />}
                <div className='pointer-events-none flex max-w-full min-w-0 flex-col gap-1.5 text-center' id={`${id}-archive-hint`} aria-live='polite'>
                  <span className='font-medium wrap-anywhere' title={archive?.name}>
                    {dragging ? t('cores.import.dropArchive') : archive?.name ?? t('cores.import.dragArchive')}
                  </span>
                  <span className='text-xs text-muted-foreground'>
                    {t(archive ? 'cores.import.replaceArchive' : 'cores.import.chooseArchive')}
                    {' · .tar.gz / .tgz'}
                  </span>
                </div>
                <Input
                  id={`${id}-archive`}
                  className='absolute inset-0 size-full cursor-pointer opacity-0 disabled:cursor-default'
                  type='file'
                  accept='.tar.gz,.tgz'
                  disabled={busy}
                  aria-invalid={!!fileError}
                  aria-describedby={fileError ? `${id}-archive-hint ${id}-archive-error` : `${id}-archive-hint`}
                  onChange={(event) => {
                    selectArchive(Array.from(event.target.files ?? []));
                    event.target.value = '';
                  }}
                />
              </div>
              {fileError && <ErrorNotice id={`${id}-archive-error`} error={t(`cores.import.${fileError}`)} />}
            </Field>
            <Field orientation='horizontal' data-disabled={busy}>
              <FieldLabel htmlFor={`${id}-version`}>{t('cores.import.version')}</FieldLabel>
              <Input
                id={`${id}-version`}
                className='w-40'
                value={version}
                disabled={busy}
                autoComplete='off'
                spellCheck={false}
                onChange={(event) => setSelection({ archive, version: event.target.value })}
                placeholder='1.14.0'
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type='button' variant='outline' disabled={busy} onClick={onClose}>
              {t('cores.confirm.cancel')}
            </Button>
            <Button type='submit' disabled={busy || !valid}>
              {busy && <Spinner data-icon='inline-start' aria-hidden='true' />}
              {t(busy ? 'cores.import.importing' : 'cores.import.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

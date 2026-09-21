import { useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CloudUpload, FileArchive } from 'lucide-react';

import type { CoreImportUpload } from '@/api/api-client';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Spinner } from '@/components/ui/spinner';
import { ErrorNotice } from '@/components/error-notice';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

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
      <DialogContent className='sm:max-w-[27.5rem]' showCloseButton={!busy}>
        <DialogHeader className='gap-1.5'>
          <DialogTitle>{t('cores.import.title')}</DialogTitle>
          <DialogDescription>{t('cores.import.description')}</DialogDescription>
        </DialogHeader>
        <form
          className='flex min-w-0 flex-col gap-4'
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
                className='core-import-dropzone'
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
                <div className='core-import-dropzone__copy' id={`${id}-archive-hint`} aria-live='polite'>
                  <span className='core-import-dropzone__title' title={archive?.name}>
                    {dragging ? t('cores.import.dropArchive') : archive?.name ?? t('cores.import.dragArchive')}
                  </span>
                  <span className='core-import-dropzone__hint'>
                    {t(archive ? 'cores.import.replaceArchive' : 'cores.import.chooseArchive')}
                    {' · .tar.gz / .tgz'}
                  </span>
                </div>
                <Input
                  id={`${id}-archive`}
                  className='core-import-dropzone__input'
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

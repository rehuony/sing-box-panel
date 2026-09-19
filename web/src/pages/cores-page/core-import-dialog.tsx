import { useState } from 'react';
import { useTranslation } from 'react-i18next';

import type { CoreImportUpload } from '@/api/api-client';

import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
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
  const [archive, setArchive] = useState<File | null>(null);
  const [version, setVersion] = useState('');
  const [description, setDescription] = useState('');
  const [variant, setVariant] = useState('plain');
  const [busy, setBusy] = useState(false);
  return (
    <Dialog open onOpenChange={(open) => !open && !busy && onClose()}>
      <DialogContent className='core-import-dialog'>
        <DialogHeader>
          <DialogTitle>{t('cores.import.title')}</DialogTitle>
          <DialogDescription className='sr-only'>{t('cores.import.description')}</DialogDescription>
        </DialogHeader>
        <div className='core-import-form'>
          <label>
            {t('cores.import.archive')}
            <Input
              type='file'
              accept='.tar.gz,.tgz,.zip'
              onChange={(event) => setArchive(event.target.files?.[0] ?? null)}
            />
          </label>
          <label>
            {t('cores.import.version')}
            <Input
              value={version}
              onChange={(event) => setVersion(event.target.value)}
              placeholder='1.14.0'
            />
          </label>
          <label>
            {t('cores.import.descriptionLabel')}
            <Input value={description} onChange={(event) => setDescription(event.target.value)} />
          </label>
          <label>
            {t('cores.import.variant')}
            <Input value={variant} onChange={(event) => setVariant(event.target.value)} />
          </label>
        </div>
        <DialogFooter>
          <Button variant='secondary' disabled={busy} onClick={onClose}>
            {t('cores.confirm.cancel')}
          </Button>
          <Button
            variant='secondary'
            disabled={
              busy
              || !archive
              || !/^\d+\.\d+\.\d+$/.test(version)
              || !description.trim()
              || !variant.trim()
            }
            onClick={async () => {
              if (!archive) return;
              setBusy(true);
              try {
                if (
                  await onImport({
                    archive,
                    exactVersion: version,
                    sourceDescription: description.trim(),
                    variant: variant.trim(),
                    architecture,
                  })
                ) {
                  onClose();
                }
              } finally {
                setBusy(false);
              }
            }}
          >
            {t('cores.import.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

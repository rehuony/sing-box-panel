import { Copy } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';

export function ConfigurationJsonPreview({ value }: { value: string }) {
  const { t } = useTranslation();
  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      toast.add({ title: t('configuration.managed.jsonCopied'), type: 'success' });
    } catch {
      toast.add({ title: t('configuration.managed.jsonCopyFailed'), type: 'error' });
    }
  }
  return (
    <div className='configuration-json-preview'>
      <div className='configuration-json-preview__toolbar'>
        <Button aria-label={t('configuration.managed.copyJson')} title={t('configuration.managed.copyJson')} onClick={() => void copy()} size='icon-sm' type='button' variant='ghost'>
          <Copy aria-hidden='true' />
        </Button>
      </div>
      <pre className='configuration-entity-json' aria-label={t('configuration.managed.json')} tabIndex={0}>{value}</pre>
    </div>
  );
}

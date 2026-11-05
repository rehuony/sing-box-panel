import { Copy } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { toast } from '@shared/components/ui/toast-manager';

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
    <div className='relative flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-[var(--radius-control)] border bg-[var(--color-surface-raised)] text-foreground'>
      <div className='absolute top-2 right-2 z-1 flex'>
        <Button aria-label={t('configuration.managed.copyJson')} title={t('configuration.managed.copyJson')} onClick={() => void copy()} size='icon-sm' type='button' variant='ghost'>
          <Copy aria-hidden='true' />
        </Button>
      </div>
      <pre className='m-0 min-h-0 min-w-0 flex-1 overflow-auto overscroll-contain p-4 font-mono text-xs leading-relaxed whitespace-pre' aria-label={t('configuration.managed.json')} tabIndex={0}>{value}</pre>
    </div>
  );
}

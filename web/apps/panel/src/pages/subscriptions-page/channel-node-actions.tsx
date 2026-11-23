import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { Separator } from '@shared/components/ui/separator';
import { CirclePlus, ListChecks, Trash2, X } from 'lucide-react';

interface Props {
  count: number;
  disabled?: boolean;
  onAdd?: () => void;
  onClear: () => void;
  canSelectAll: boolean;
  onRemove?: () => void;
  onSelectAll: () => void;
}

export function ChannelNodeActions({ count, disabled, canSelectAll, onClear, onSelectAll, onRemove, onAdd }: Props) {
  const { t } = useTranslation();
  return (
    <div className='flex min-w-0 flex-none flex-wrap items-center gap-2 flex-nowrap'>
      {count > 0 && (
        <div className='flex flex-none items-center gap-1 rounded-[var(--radius-card)] bg-[var(--color-surface-raised)] ps-2.5 [&_[data-slot=separator]]:h-4 [&_[data-slot=separator]]:self-center [&_[data-slot=separator]]:border-s [&_[data-slot=separator]]:border-dashed [&_[data-slot=separator]]:bg-transparent'>
          <span className='me-1.5 min-w-[1ch] text-xs text-muted-foreground tabular-nums' role='status' aria-label={t('channels.selectedNodes', { count })}>{count}</span>
          <Separator orientation='vertical' aria-hidden='true' />
          <Button size='icon-sm' variant='ghost' disabled={disabled} aria-label={t('channels.clearNodes')} title={t('channels.clearNodes')} onClick={onClear}>
            <X />
          </Button>
          {onRemove && (
            <Button size='icon-sm' variant='ghost' className='text-[var(--color-danger)]' disabled={disabled} aria-label={t('channels.removeNodes')} title={t('channels.removeNodes')} onClick={onRemove}>
              <Trash2 />
            </Button>
          )}
        </div>
      )}
      <Button size='icon-sm' variant='ghost' disabled={disabled || !canSelectAll} aria-label={t('channels.selectAll')} title={t('channels.selectAll')} onClick={onSelectAll}>
        <ListChecks />
      </Button>
      {onAdd && (
        <Button size='icon-sm' variant='ghost' disabled={disabled} aria-label={t('channels.addNodes')} title={t('channels.addNodes')} onClick={onAdd}>
          <CirclePlus />
        </Button>
      )}
    </div>
  );
}

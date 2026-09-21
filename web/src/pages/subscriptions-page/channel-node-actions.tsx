import { useTranslation } from 'react-i18next';
import { CirclePlus, ListChecks, Trash2, X } from 'lucide-react';

import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';

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
    <div className='channel-selection-actions channel-node-actions'>
      {count > 0 && (
        <div className='channel-node-selection'>
          <span className='channel-node-selection-count' role='status' aria-label={t('channels.selectedNodes', { count })}>{count}</span>
          <Separator orientation='vertical' aria-hidden='true' />
          <Button size='icon-sm' variant='ghost' disabled={disabled} aria-label={t('channels.clearNodes')} title={t('channels.clearNodes')} onClick={onClear}>
            <X />
          </Button>
          {onRemove && (
            <Button size='icon-sm' variant='ghost' className='channel-delete-action' disabled={disabled} aria-label={t('channels.removeNodes')} title={t('channels.removeNodes')} onClick={onRemove}>
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

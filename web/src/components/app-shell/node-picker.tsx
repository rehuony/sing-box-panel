import { useTranslation } from 'react-i18next';

import { useNodeSelection } from '@/stores/node-selection.store';
import { useUnsavedChangesContext } from '@/stores/unsaved-changes.store';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

/** Chooses the node that the node-scoped pages manage. */
export function NodePicker() {
  const { t } = useTranslation();
  const { confirmNavigation } = useUnsavedChangesContext();
  const { nodes, select, selected, status } = useNodeSelection();

  if (status === 'error') return <p className='panel-node-picker__note'>{t('nodes.loadFailed')}</p>;
  if (status === 'success' && nodes.length === 0) return <p className='panel-node-picker__note'>{t('nodes.empty')}</p>;
  return (
    <Select
      value={selected?.id ?? null}
      onValueChange={(value) => {
        if (typeof value === 'string' && value !== selected?.id) confirmNavigation(() => select(value));
      }}
    >
      <SelectTrigger aria-label={t('nodes.label')} className='panel-node-picker'>
        <SelectValue>{selected?.name ?? t('shell.loading.title')}</SelectValue>
      </SelectTrigger>
      <SelectContent>
        {nodes.map(node => (
          <SelectItem key={node.id} value={node.id}>
            <span className='panel-node-picker__name'>{node.name}</span>
            <span className='panel-node-picker__state'>{t(`nodes.state.${node.state}`)}</span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

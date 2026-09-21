import { useState } from 'react';
import { Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { ChannelRuleGroup, SubscriptionFormat, SubscriptionNodeSummary } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';

import type { ChannelNodeCard } from './channel-node-cards';

import { candidateOrder } from './channel-node-order';
import { ChannelNodeCards } from './channel-node-cards';
import { ChannelNodeActions } from './channel-node-actions';
import { reorderVisibleNodes } from './subscription-node-order';

interface Props {
  onClose: () => void;
  group: ChannelRuleGroup;
  format: SubscriptionFormat;
  nodes: SubscriptionNodeSummary[];
  onAdd: (ids: string[], builtins: ChannelRuleGroup['builtin_nodes'], order: string[]) => void;
}

export function ChannelNodePicker({ nodes, group, format, onClose, onAdd }: Props) {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<string[]>([]);
  const [dragging, setDragging] = useState(false);
  const [order, setOrder] = useState(() => [
    'builtin:direct', 'builtin:reject', ...nodes.filter((node) => !node.hidden && node.available).map((node) => `node:${node.id}`),
  ]);
  const matches = (text: string) => text.toLowerCase().includes(search.trim().toLowerCase());
  const added = new Set(candidateOrder(group));
  const catalog = new Map(nodes.filter((node) => !node.hidden && node.available).map((node) => [`node:${node.id}`, node]));
  const items: ChannelNodeCard[] = order.flatMap((id) => {
    const builtin = id === 'builtin:direct' ? 'direct' : id === 'builtin:reject' ? 'reject' : undefined;
    const node = catalog.get(id);
    if (!builtin && !node) return [];
    const unsupported = builtin === 'reject' && format !== 'mihomo';
    const label = builtin ? builtin.toUpperCase() : `${node!.name} ${node!.source_name}`;
    if (!matches(builtin ? `${builtin} ${t(`channels.${builtin}`)}` : `${label} ${node!.type}`)) return [];
    return [{
      id, label, node, builtin, selected: added.has(id) || selected.includes(id),
      disabled: added.has(id) || unsupported, unavailable: unsupported,
    }];
  });
  const count = selected.length;
  const visible = items.map((item) => item.id);
  function add() {
    const chosen = order.filter((id) => selected.includes(id));
    onAdd(
      chosen.filter((id) => id.startsWith('node:')).map((id) => id.slice(5)),
      chosen.flatMap((id) => id === 'builtin:direct' ? ['direct' as const] : id === 'builtin:reject' ? ['reject' as const] : []),
      chosen,
    );
  }
  return (
    <Dialog open onOpenChange={(open, details) => {
      if (dragging) {
        details.cancel();
        // Let DND-KIT receive Escape so it can cancel the drag itself.
        details.allowPropagation();
      } else if (!open) {
        onClose();
      }
    }}>
      <DialogContent
        className='channel-node-picker'
        showCloseButton={false}
        onKeyDown={(event) => {
          // The dialog normally traps arrow keys before DND-KIT's document listener.
          if (dragging && (event.key.startsWith('Arrow') || event.key === 'Escape')) event.preventBaseUIHandler();
        }}
      >
        <div className='channel-node-picker-header'>
          <DialogHeader>
            <DialogTitle>{t('channels.addNodes')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('channels.pickNodesHint')}</DialogDescription>
          </DialogHeader>
          <div className='channel-node-picker-toolbar'>
            <ChannelNodeActions
              count={count}
              canSelectAll={items.some((item) => !item.disabled && !item.selected)}
              onClear={() => setSelected([])}
              onSelectAll={() => setSelected((current) => [...new Set([
                ...current, ...items.filter((item) => !item.disabled).map((item) => item.id),
              ])])}
            />
            <div className='subscription-search'>
              <Search aria-hidden='true' />
              <input aria-label={t('channels.searchNodes')} placeholder={t('channels.searchNodes')} value={search} onChange={(event) => setSearch(event.target.value)} />
            </div>
          </div>
        </div>
        <div className='channel-node-picker-scroll'>
          <ChannelNodeCards
            items={items}
            onToggle={(id) => setSelected((current) =>
              current.includes(id) ? current.filter((key) => key !== id) : [...current, id])}
            onMove={(active, over) => setOrder(reorderVisibleNodes(order, visible, active, over))}
            onDraggingChange={setDragging}
          />
          {!items.length && <p className='subscription-empty'>{t('channels.noMatchingNodes')}</p>}
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={onClose}>{t('common.cancel')}</Button>
          <Button disabled={!count} onClick={add}>{t('channels.addSelectedNodes', { count })}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

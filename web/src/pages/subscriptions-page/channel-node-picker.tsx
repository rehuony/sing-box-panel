import { useState } from 'react';
import { Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';

import type { ChannelRuleGroup, SubscriptionFormat, SubscriptionNodeSummary } from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';

import type { ChannelNodeCard } from './channel-node-cards';

import { createsGroupCycle } from './channel-policy';
import { candidateOrder } from './channel-node-order';
import { ChannelNodeCards } from './channel-node-cards';
import { ChannelNodeActions } from './channel-node-actions';
import { reorderVisibleNodes } from './subscription-node-order';

interface Props {
  onClose: () => void;
  group: ChannelRuleGroup;
  format: SubscriptionFormat;
  groups: ChannelRuleGroup[];
  nodes: SubscriptionNodeSummary[];
  onAdd: (ids: string[], builtins: ChannelRuleGroup['builtin_nodes'], order: string[], groupIDs: string[]) => void;
}

export function ChannelNodePicker({ nodes, group, groups, format, onClose, onAdd }: Props) {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<string[]>([]);
  const [dragging, setDragging] = useState(false);
  const [order, setOrder] = useState(() => [
    'builtin:direct', 'builtin:reject', ...nodes.filter((node) => !node.hidden && node.available).map((node) => `node:${node.id}`),
    ...groups.filter(value => value.id !== group.id).map(value => `group:${value.id}`),
  ]);
  const matches = (text: string) => text.toLowerCase().includes(search.trim().toLowerCase());
  const added = new Set(candidateOrder(group));
  const catalog = new Map(nodes.filter((node) => !node.hidden && node.available).map((node) => [`node:${node.id}`, node]));
  const groupCatalog = new Map(groups.map(value => [`group:${value.id}`, value]));
  const items: ChannelNodeCard[] = order.flatMap((id) => {
    const builtin = id === 'builtin:direct' ? 'direct' : id === 'builtin:reject' ? 'reject' : undefined;
    const node = catalog.get(id);
    const candidate = groupCatalog.get(id);
    if (!builtin && !node && !candidate) return [];
    const unsupported = (builtin === 'reject' && format === 'sing-box')
      || (Boolean(builtin) && format === 'loon' && group.type !== 'select');
    const label = builtin ? builtin.toUpperCase() : candidate ? candidate.name : `${node!.name} ${node!.source_name}`;
    const disabledReason = added.has(id)
      ? t('channels.alreadyAdded')
      : candidate && !candidate.enabled
        ? t('channels.disabled')
        : candidate && createsGroupCycle(groups, group.id, candidate.id)
          ? t('channels.cyclicGroupReference')
          : unsupported ? t('channels.unavailable') : undefined;
    if (!matches(builtin ? `${builtin} ${t(`channels.${builtin}`)}` : `${label} ${candidate ? `${t('channels.strategyGroup')} ${t(`channels.groupTypes.${candidate.type}`)}` : node!.type}`)) return [];
    return [{
      id, label, node, builtin, group: candidate, disabledReason, selected: added.has(id) || selected.includes(id),
      disabled: Boolean(disabledReason), unavailable: unsupported || Boolean(candidate && !candidate.enabled),
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
      chosen.filter(id => id.startsWith('group:')).map(id => id.slice(6)),
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

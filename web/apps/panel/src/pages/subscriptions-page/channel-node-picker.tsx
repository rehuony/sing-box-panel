import { Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@shared/components/ui/button';
import { useCallback, useDeferredValue, useMemo, useState } from 'react';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@shared/components/ui/input-group';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@shared/components/ui/dialog';

import type { ChannelGroupMember, ChannelRuleGroup, SubscriptionFormat, SubscriptionNode } from '@/api/api-client';

import type { ChannelNodeCard } from './channel-node-cards';

import { BUILTIN_EXIT_NAMES } from './channel-exits';
import { nodeOrigin } from './subscription-node-pool';
import { ChannelNodeCards } from './channel-node-cards';
import { ChannelNodeActions } from './channel-node-actions';
import { reorderVisibleNodes } from './subscription-node-order';
import { SubscriptionContentState } from './subscription-content-state';
import { createsGroupCycle, groupTypeName, memberKey } from './channel-policy';

interface Props {
  onClose: () => void;
  group: ChannelRuleGroup;
  /** The pool in its saved order. */
  nodes: SubscriptionNode[];
  format: SubscriptionFormat;
  groups: ChannelRuleGroup[];
  /** The chosen candidates in the order the picker shows them. */
  onAdd: (members: ChannelGroupMember[]) => void;
}

const BUILTINS = ['direct', 'reject'] as const;

type Candidate = ChannelGroupMember & { node?: SubscriptionNode; group?: ChannelRuleGroup };

/**
 * Picks candidates to add to a group: the built-in exits DIRECT and REJECT,
 * the channel's other groups in their order, then the pool's nodes in pool
 * order. A built-in the client lacks for the group, such as REJECT for
 * sing-box, is left out unless the group holds it; candidates already in the
 * group, disabled or making a cycle stay listed but cannot be chosen. The
 * chosen ones join in the order shown here.
 */
export function ChannelNodePicker({ nodes, group, groups, format, onClose, onAdd }: Props) {
  const { t } = useTranslation();
  const [search, setSearch] = useState('');
  const deferredSearch = useDeferredValue(search);
  const [selected, setSelected] = useState<string[]>([]);
  const [dragging, setDragging] = useState(false);
  const catalog = useMemo(() => new Map<string, Candidate>([
    ...BUILTINS.map(id => [`builtin:${id}`, { kind: 'builtin' as const, id }] as const),
    ...groups.filter(value => value.id !== group.id).map(value => [`group:${value.id}`, { kind: 'group' as const, id: value.id, group: value }] as const),
    ...nodes.map(node => [`node:${node.id}`, { kind: 'node' as const, id: node.id, node }] as const),
  ]), [group.id, groups, nodes]);
  const [order, setOrder] = useState(() => [...catalog.keys()]);
  const query = deferredSearch.trim().toLowerCase();
  const chosen = useMemo(() => new Set(selected), [selected]);
  const items = useMemo(() => {
    const added = new Set(group.members.map(memberKey));
    return order.flatMap((key): ChannelNodeCard[] => {
      const entry = catalog.get(key);
      if (!entry) return [];
      const builtin = entry.kind === 'builtin' ? entry.id as typeof BUILTINS[number] : undefined;
      const candidate = entry.group;
      const unsupported = (builtin === 'reject' && format === 'sing-box')
        || (builtin !== undefined && format === 'loon' && group.type !== 'select');
      if (unsupported && !added.has(key)) return [];
      let label = builtin ? BUILTIN_EXIT_NAMES[builtin] : candidate?.name ?? '';
      if (entry.node) label = entry.node.display_name;
      let searched = label;
      if (entry.node) searched = `${label} ${nodeOrigin(entry.node)} ${entry.node.protocol}`;
      if (candidate) searched = `${label} ${t('channels.strategyGroup')} ${groupTypeName(candidate.type, format)}`;
      if (query && !searched.toLowerCase().includes(query)) return [];
      let disabledReason: string | undefined;
      if (added.has(key)) disabledReason = t('channels.alreadyAdded');
      else if (candidate && !candidate.enabled) disabledReason = t('channels.disabled');
      else if (candidate && createsGroupCycle(groups, group.id, candidate.id)) disabledReason = t('channels.cyclicGroupReference');
      return [{
        id: key, label, node: entry.node, builtin, group: candidate, disabledReason,
        selected: added.has(key) || chosen.has(key),
        disabled: Boolean(disabledReason), unavailable: unsupported || Boolean(candidate && !candidate.enabled),
      }];
    });
  }, [catalog, chosen, format, group, groups, order, query, t]);
  const count = selected.length;
  const visible = items.map((item) => item.id);
  const toggle = useCallback((id: string) => setSelected(current =>
    current.includes(id) ? current.filter(key => key !== id) : [...current, id]), []);
  function add() {
    onAdd(order.filter(key => chosen.has(key)).flatMap((key) => {
      const entry = catalog.get(key);
      return entry ? [{ kind: entry.kind, id: entry.id }] : [];
    }));
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
        className='flex h-[min(640px,calc(100dvh-32px))] flex-col overflow-hidden max-[600px]:p-4'
        size='workspace'
        showCloseButton={false}
        onKeyDown={(event) => {
          // The dialog normally traps arrow keys before DND-KIT's document listener.
          if (dragging && (event.key.startsWith('Arrow') || event.key === 'Escape')) event.preventBaseUIHandler();
        }}
      >
        <div className='flex min-w-0 flex-none items-center justify-between gap-4 max-[600px]:gap-1.5 [&_[data-slot=dialog-header]]:flex-none [&_[data-slot=dialog-title]]:pr-0 [&_[data-slot=dialog-title]]:whitespace-nowrap max-[600px]:[&_[data-slot=dialog-title]]:text-sm'>
          <DialogHeader>
            <DialogTitle>{t('channels.addNodes')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('channels.pickNodesHint')}</DialogDescription>
          </DialogHeader>
          <div className='flex min-w-0 items-center justify-end gap-2 max-[600px]:gap-1.5'>
            <ChannelNodeActions
              count={count}
              canSelectAll={items.some((item) => !item.disabled && !item.selected)}
              onClear={() => setSelected([])}
              onSelectAll={() => setSelected((current) => [...new Set([
                ...current, ...items.filter((item) => !item.disabled).map((item) => item.id),
              ])])}
            />
            <InputGroup className='h-(--control-height-sm) w-60 min-w-0 flex-[0_1_240px] max-[600px]:[&>[data-slot=input-group-addon]]:hidden'>
              <InputGroupAddon><Search aria-hidden='true' /></InputGroupAddon>
              <InputGroupInput className='h-full min-h-0' aria-label={t('channels.searchNodes')} placeholder={t('channels.searchNodes')} value={search} onChange={(event) => setSearch(event.target.value)} />
            </InputGroup>
          </div>
        </div>
        <div className='min-h-0 flex-1 overflow-y-auto overscroll-contain p-0.5 flex flex-col'>
          <ChannelNodeCards
            format={format}
            items={items}
            onToggle={toggle}
            onMove={(active, over) => setOrder(reorderVisibleNodes(order, visible, active, over))}
            onDraggingChange={setDragging}
          />
          {!items.length && <SubscriptionContentState kind='search' title={t('channels.noMatchingNodes')} />}
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={onClose}>{t('common.cancel')}</Button>
          <Button disabled={!count} onClick={add}>{t('channels.addSelectedNodes', { count })}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

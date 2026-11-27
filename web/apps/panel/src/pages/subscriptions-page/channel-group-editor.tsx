import { useState } from 'react';
import { Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { ScrollArea } from '@shared/components/ui/scroll-area';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@shared/components/ui/tabs';
import { InputGroup, InputGroupAddon, InputGroupInput } from '@shared/components/ui/input-group';

import type {
  ChannelRoute,
  ChannelRuleGroup,
  SubscriptionChannelConfig,
  SubscriptionFormat,
  SubscriptionNode,
  SubscriptionRuleSet,
} from '@/api/api-client';

import type { ChannelNodeCard } from './channel-node-cards';

import { BUILTIN_EXIT_NAMES } from './channel-exits';
import { nodeOrigin } from './subscription-node-pool';
import { ChannelNodePicker } from './channel-node-picker';
import { ChannelNodeActions } from './channel-node-actions';
import { reorderVisibleNodes } from './subscription-node-order';
import { SubscriptionContentState } from './subscription-content-state';
import { ChannelRouteAdd, ChannelRouteList } from './channel-route-list';
import { groupTypeName, matchedNodes, memberKey } from './channel-policy';
import { ChannelMatchedCards, ChannelNodeCards } from './channel-node-cards';

export type ChannelGroupTab = 'nodes' | 'rules';

interface Props {
  busy: boolean;
  tab: ChannelGroupTab;
  group: ChannelRuleGroup;
  /** The pool in its saved order. */
  nodes: SubscriptionNode[];
  format: SubscriptionFormat;
  config: SubscriptionChannelConfig;
  onChange: (group: ChannelRuleGroup) => void;
  onTabChange: (tab: ChannelGroupTab) => void;
  onRoutesChange: (routes: ChannelRoute[]) => void;
  ruleSets: readonly SubscriptionRuleSet[] | undefined;
}

/**
 * One strategy group: its candidates, the members in their order and then
 * the pool nodes its match takes, or the channel's exit rules that leave
 * through it.
 */
export function ChannelGroupEditor({
  busy, config, format, group, nodes, onChange, onRoutesChange, onTabChange, ruleSets, tab,
}: Props) {
  const { t } = useTranslation();
  const [addingNodes, setAddingNodes] = useState(false);
  const [search, setSearch] = useState('');
  const [selected, setSelected] = useState<string[]>([]);
  const query = search.trim().toLowerCase();
  const matches = (value: string) => value.toLowerCase().includes(query);
  const order = group.members.map(memberKey);
  const selectedIDs = new Set(order.filter((id) => selected.includes(id)));
  const groupCatalog = new Map(config.groups.map(value => [value.id, value]));
  const catalog = new Map(nodes.map((node) => [node.id, node]));
  const cards: ChannelNodeCard[] = group.members.flatMap((member) => {
    const id = memberKey(member);
    const builtin = member.kind === 'builtin' ? member.id as 'direct' | 'reject' : undefined;
    const node = member.kind === 'node' ? catalog.get(member.id) : undefined;
    const candidate = member.kind === 'group' ? groupCatalog.get(member.id) : undefined;
    let label = node?.display_name ?? candidate?.name ?? t('channels.missingNode');
    if (builtin) label = BUILTIN_EXIT_NAMES[builtin];
    let searched = `${label} ${node ? `${nodeOrigin(node)} ${node.protocol}` : member.id}`;
    if (builtin) searched = label;
    if (candidate) searched = `${label} ${t('channels.strategyGroup')} ${groupTypeName(candidate.type, format)}`;
    if (!matches(searched)) return [];
    let unavailable = !node;
    if (builtin) unavailable = (format === 'sing-box' && builtin === 'reject') || (format === 'loon' && group.type !== 'select');
    if (member.kind === 'group') unavailable = !candidate?.enabled;
    return [{ id, label, node, builtin, group: candidate, selected: selectedIDs.has(id), disabled: busy, unavailable }];
  });
  const matched = matchedNodes(group, nodes).filter(node => matches(`${node.display_name} ${nodeOrigin(node)} ${node.protocol}`));

  return (
    <section className='flex min-h-0 min-w-0 flex-1 flex-col' aria-label={t('channels.editGroup')}>
      {addingNodes && (
        <ChannelNodePicker
          nodes={nodes}
          group={group}
          groups={config.groups}
          format={format}
          onClose={() => setAddingNodes(false)}
          onAdd={(members) => {
            const present = new Set(order);
            const added = members.filter(member => !present.has(memberKey(member)));
            onChange({ ...group, members: [...group.members, ...added] });
            setAddingNodes(false);
          }} />
      )}
      <Tabs value={tab} onValueChange={(value) => {
        if (value === 'nodes' || value === 'rules') onTabChange(value);
      }} className='min-h-0 min-w-0 flex-1 gap-0'>
        <div className='flex min-h-12 min-w-0 flex-none flex-wrap items-center gap-2 border-b px-3 py-1.5'>
          <TabsList aria-label={t('channels.editGroup')} className='flex-none' variant='pill'>
            <TabsTrigger value='nodes'>{t('channels.candidates')}</TabsTrigger>
            <TabsTrigger value='rules'>{t('channels.routes')}</TabsTrigger>
          </TabsList>
          <span className='flex-1' />
          {tab === 'nodes'
            ? (
                <div className='flex min-w-0 flex-wrap items-center gap-2 max-[720px]:flex-[1_1_100%]'>
                  <ChannelNodeActions
                    count={selectedIDs.size}
                    disabled={busy}
                    canSelectAll={cards.some((card) => !card.selected)}
                    onClear={() => setSelected([])}
                    onSelectAll={() => setSelected((current) => [...new Set([
                      ...current, ...cards.map((card) => card.id),
                    ])])}
                    onRemove={() => {
                      const kept = group.members.filter(member => !selectedIDs.has(memberKey(member)));
                      onChange({ ...group, members: kept });
                      setSelected([]);
                    }}
                    onAdd={() => setAddingNodes(true)}
                  />
                  <InputGroup className='h-(--control-height-sm) w-60 min-w-30 max-[900px]:w-50 max-[720px]:flex-1'>
                    <InputGroupAddon><Search aria-hidden='true' /></InputGroupAddon>
                    <InputGroupInput className='h-full min-h-0' aria-label={t('channels.searchNodes')} placeholder={t('channels.searchNodes')} value={search} onChange={(event) => setSearch(event.target.value)} />
                  </InputGroup>
                </div>
              )
            : (
                <ChannelRouteAdd
                  busy={busy}
                  exit={{ kind: 'group', id: group.id }}
                  exitName={group.name || t('channels.groupName')}
                  format={format}
                  onAdd={added => onRoutesChange([...config.routes, ...added])}
                  routes={config.routes}
                  ruleSets={ruleSets}
                />
              )}
        </div>
        <TabsContent value='nodes' className='flex min-h-0 flex-1 flex-col'>
          {cards.length || matched.length
            ? (
                <ScrollArea className='min-h-0 flex-1'>
                  <fieldset className='flex min-w-0 flex-col gap-4 p-4' disabled={busy}>
                    <ChannelNodeCards
                      format={format}
                      items={cards}
                      onToggle={(id) => setSelected((current) =>
                        current.includes(id) ? current.filter((value) => value !== id) : [...current, id])}
                      onMove={(active, over) => {
                        const byKey = new Map(group.members.map(member => [memberKey(member), member]));
                        const next = reorderVisibleNodes(order, cards.map((card) => card.id), active, over);
                        onChange({ ...group, members: next.map(key => byKey.get(key)!) });
                      }}
                    />
                    <ChannelMatchedCards format={format} nodes={matched} />
                  </fieldset>
                </ScrollArea>
              )
            : <SubscriptionContentState kind={search ? 'search' : 'candidates'} title={t(search ? 'channels.noMatchingNodes' : 'channels.emptyCandidates')} />}
        </TabsContent>
        <TabsContent value='rules' className='flex min-h-0 flex-1 flex-col'>
          <ChannelRouteList
            busy={busy}
            config={config}
            format={format}
            groupID={group.id}
            nodes={nodes}
            onRoutesChange={onRoutesChange}
            ruleSets={ruleSets}
          />
        </TabsContent>
      </Tabs>
    </section>
  );
}

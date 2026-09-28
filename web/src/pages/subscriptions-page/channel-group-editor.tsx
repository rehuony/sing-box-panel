import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { CirclePlus, Pencil, Search, Trash2 } from 'lucide-react';

import type {
  ChannelRouteExit,
  ChannelRule,
  ChannelRuleGroup,
  SubscriptionFormat,
  SubscriptionNodeSummary,
} from '@/api/api-client';

import { Button } from '@/components/ui/button';
import { ErrorNotice } from '@/components/error-notice';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';

import type { ChannelNodeCard } from './channel-node-cards';

import { candidateOrder } from './channel-node-order';
import { ChannelNodeCards } from './channel-node-cards';
import { ChannelNodePicker } from './channel-node-picker';
import { ChannelRuleEditor } from './channel-rule-editor';
import { ChannelNodeActions } from './channel-node-actions';
import { reorderVisibleNodes } from './subscription-node-order';
import { SubscriptionContentState } from './subscription-content-state';
import { compareChannelRules, ruleFormats, updateGroupCandidates } from './channel-policy';

interface Props {
  busy: boolean;
  nextSortIndex: number;
  group: ChannelRuleGroup;
  format: SubscriptionFormat;
  groups: ChannelRuleGroup[];
  nodes: SubscriptionNodeSummary[];
  onChange: (group: ChannelRuleGroup) => void;
}
export function ChannelGroupEditor({ group, groups, nodes, format, busy, onChange, nextSortIndex }: Props) {
  const { t } = useTranslation();
  const [addingNodes, setAddingNodes] = useState(false);
  const [rule, setRule] = useState<ChannelRule | null>(null);
  const [search, setSearch] = useState('');
  const [tab, setTab] = useState('nodes');
  const [selected, setSelected] = useState<string[]>([]);
  const matches = (value: string) => value.toLowerCase().includes(search.trim().toLowerCase());
  const builtins = group.builtin_nodes;
  const order = candidateOrder(group);
  const selectedIDs = new Set(order.filter((id) => selected.includes(id)));
  const groupCatalog = new Map(groups.map(value => [value.id, value]));
  const catalog = new Map(nodes.map((node) => [node.id, node]));
  const cards: ChannelNodeCard[] = order.flatMap((id) => {
    const builtin = id === 'builtin:direct' ? 'direct' : id === 'builtin:reject' ? 'reject' : undefined;
    const node = id.startsWith('node:') ? catalog.get(id.slice(5)) : undefined;
    const candidate = id.startsWith('group:') ? groupCatalog.get(id.slice(6)) : undefined;
    const label = builtin ? builtin.toUpperCase() : candidate ? candidate.name : node ? `${node.name} ${node.source_name}` : t('channels.missingNode');
    if (!matches(builtin ? `${label} ${t(`channels.${builtin}`)}` : `${label} ${candidate ? `${t('channels.strategyGroup')} ${t(`channels.groupTypes.${candidate.type}`)}` : node?.type ?? id}`)) return [];
    return [{
      id, label, node, builtin, group: candidate, selected: selectedIDs.has(id), disabled: busy,
      unavailable: builtin
        ? (format === 'sing-box' && builtin === 'reject') || (format === 'loon' && group.type !== 'select')
        : candidate ? !candidate.enabled : !node || node.hidden || !node.available,
    }];
  });
  function updateCandidates(
    node_ids: string[], builtin_nodes = builtins, candidate_order = order, group_ids = group.group_ids ?? [],
  ) {
    onChange(updateGroupCandidates(group, node_ids, builtin_nodes, candidate_order, group_ids));
  }

  function exitLabel(exit: ChannelRouteExit) {
    if (exit.kind === 'node') return nodes.find((node) => node.id === exit.id)?.name ?? t('channels.missingNode');
    return t(exit.kind === 'group-default' ? 'channels.follow' : exit.kind === 'reject' ? 'channels.reject' : 'channels.direct');
  }
  function newRule(remote: boolean) {
    setRule({
      id: crypto.randomUUID(),
      enabled: true,
      sort_index: nextSortIndex,
      kind: remote ? 'remote' : 'domain_suffix',
      exit: { kind: 'group-default' },
      ...(remote
        ? {
            remote: {
              name: '',
              url: '',
              format: ruleFormats(format)[0],
              behavior: format === 'mihomo' ? 'domain' : undefined,
              accelerated: false,
            },
          }
        : { value: '' }),
    });
  }
  return (
    <section className='channel-group-editor' aria-label={t('channels.editGroup')}>
      {addingNodes && (
        <ChannelNodePicker
          nodes={nodes}
          group={group}
          groups={groups}
          format={format}
          onClose={() => setAddingNodes(false)}
          onAdd={(ids, builtinNodes, addedOrder, groupIDs) => {
            updateCandidates(
              [...new Set([...group.node_ids, ...ids])],
              [...new Set([...builtins, ...builtinNodes])],
              [...order, ...addedOrder],
              [...new Set([...(group.group_ids ?? []), ...groupIDs])],
            );
            setAddingNodes(false);
          }} />
      )}
      <fieldset className='channel-group-fields' disabled={busy}>
        <Tabs value={tab} onValueChange={setTab} className='channel-group-tabs'>
          <div className='channel-group-toolbar'>
            <TabsList className='subscriptions-tabs' aria-label={t('channels.editGroup')}>
              <TabsTrigger value='nodes'>{t('channels.candidates')}</TabsTrigger>
              <TabsTrigger value='rules'>{t('channels.matches')}</TabsTrigger>
            </TabsList>
            {tab === 'rules' && (
              <div className='channel-selection-actions channel-group-rule-tools'>
                <Button size='sm' variant='ghost' onClick={() => newRule(false)}>
                  <CirclePlus data-icon='inline-start' />
                  {t('channels.addRule')}
                </Button>
                <Button size='sm' variant='ghost' onClick={() => newRule(true)}>
                  <CirclePlus data-icon='inline-start' />
                  {t('channels.addRemote')}
                </Button>
              </div>
            )}
            {tab === 'nodes' && (
              <div className='channel-group-node-tools'>
                <ChannelNodeActions
                  count={selectedIDs.size}
                  disabled={busy}
                  canSelectAll={cards.some((card) => !card.selected)}
                  onClear={() => setSelected([])}
                  onSelectAll={() => setSelected((current) => [...new Set([
                    ...current, ...cards.map((card) => card.id),
                  ])])}
                  onRemove={() => {
                    updateCandidates(
                      group.node_ids.filter((id) => !selectedIDs.has(`node:${id}`)),
                      builtins.filter((kind) => !selectedIDs.has(`builtin:${kind}`)),
                      order,
                      (group.group_ids ?? []).filter(id => !selectedIDs.has(`group:${id}`)),
                    );
                    setSelected([]);
                  }}
                  onAdd={() => setAddingNodes(true)}
                />
                <div className='subscription-search'>
                  <Search aria-hidden='true' />
                  <input aria-label={t('channels.searchNodes')} placeholder={t('channels.searchNodes')} value={search} onChange={(event) => setSearch(event.target.value)} />
                </div>
              </div>
            )}
          </div>
          <TabsContent value='nodes' className='channel-group-content'>
            <ChannelNodeCards
              items={cards}
              onToggle={(id) => setSelected((current) =>
                current.includes(id) ? current.filter((value) => value !== id) : [...current, id])}
              onMove={(active, over) => onChange({
                ...group, candidate_order: reorderVisibleNodes(order, cards.map((card) => card.id), active, over),
              })}
            />
            {!cards.length && (
              <SubscriptionContentState kind={search ? 'search' : 'candidates'} title={t(search ? 'channels.noMatchingNodes' : 'channels.emptyCandidates')} />
            )}
          </TabsContent>
          <TabsContent value='rules' className='channel-group-content'>
            <ul className='channel-rule-rows' aria-label={t('channels.matches')}>
              {[...group.rules].sort(compareChannelRules).map((item) => (
                <li className='channel-rule-row' key={item.id} data-disabled={!item.enabled || undefined}>
                  <div className='channel-rule-label' title={item.remote?.url ?? item.value}>
                    <span className='channel-rule-name' title={item.remote?.name ?? item.value}>{item.remote?.name ?? item.value}</span>
                    <span className='channel-rule-summary' title={[
                      `${t('channels.sortIndex')}: ${item.sort_index ?? 0}`,
                      t(item.kind === 'remote' ? 'channels.ruleSet' : `channels.${item.kind}`),
                      exitLabel(item.exit),
                      ...(!item.enabled ? [t('channels.disabled')] : []),
                    ].join(' · ')}>
                      <span>
                        {t('channels.sortIndex')}
                        :
                        {' '}
                        {item.sort_index ?? 0}
                      </span>
                      <span>{t(item.kind === 'remote' ? 'channels.ruleSet' : `channels.${item.kind}`)}</span>
                      <span>{exitLabel(item.exit)}</span>
                      {!item.enabled && <span>{t('channels.disabled')}</span>}
                    </span>
                    {item.remote && !ruleFormats(format).includes(item.remote.format) && (
                      <ErrorNotice error={t('channels.formatPending')} />
                    )}
                  </div>
                  <div className='subscription-toolbar-actions'>
                    <Button variant='ghost' size='icon-sm' aria-label={t('channels.editRule')} title={t('channels.editRule')} onClick={() => setRule(item)}><Pencil /></Button>
                    <Button variant='ghost' size='icon-sm' className='channel-delete-action' aria-label={t('channels.deleteRule')} title={t('channels.deleteRule')} onClick={() => onChange({ ...group, rules: group.rules.filter((value) => value.id !== item.id) })}><Trash2 /></Button>
                  </div>
                </li>
              ))}
            </ul>
            {!group.rules.length && (
              <SubscriptionContentState kind='rules' title={t('channels.matches')} />
            )}
          </TabsContent>
        </Tabs>
      </fieldset>
      {rule && (
        <ChannelRuleEditor
          rule={rule}
          otherRuleNames={groups.flatMap(value => value.rules
            .filter(item => item.remote && !(value.id === group.id && item.id === rule.id))
            .map(item => item.remote!.name.trim()))}
          format={format}
          onClose={() => setRule(null)}
          onSave={async (value) => onChange({
            ...group,
            rules: group.rules.some((existing) => existing.id === value.id)
              ? group.rules.map((existing) => existing.id === value.id ? value : existing)
              : [...group.rules, value],
          })}
        />
      )}
    </section>
  );
}

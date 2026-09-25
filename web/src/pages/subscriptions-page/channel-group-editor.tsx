import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ArrowDown, ArrowRight, ArrowUp, CirclePlus, Pencil, Route, Search, Trash2 } from 'lucide-react';

import type {
  ChannelRouteExit,
  ChannelRule,
  ChannelRuleGroup,
  SubscriptionFormat,
  SubscriptionNodeSummary,
} from '@/api/api-client';

import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ErrorNotice } from '@/components/error-notice';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';

import type { ChannelNodeCard } from './channel-node-cards';

import { ruleFormats } from './channel-policy';
import { candidateOrder } from './channel-node-order';
import { ChannelNodeCards } from './channel-node-cards';
import { ChannelNodePicker } from './channel-node-picker';
import { ChannelRuleEditor } from './channel-rule-editor';
import { ChannelNodeActions } from './channel-node-actions';
import { reorderVisibleNodes } from './subscription-node-order';

interface Props {
  busy: boolean;
  group: ChannelRuleGroup;
  format: SubscriptionFormat;
  nodes: SubscriptionNodeSummary[];
  onChange: (group: ChannelRuleGroup) => void;
}
export function ChannelGroupEditor({ group, nodes, format, busy, onChange }: Props) {
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
  const catalog = new Map(nodes.map((node) => [node.id, node]));
  const cards: ChannelNodeCard[] = order.flatMap((id) => {
    const builtin = id === 'builtin:direct' ? 'direct' : id === 'builtin:reject' ? 'reject' : undefined;
    const node = builtin ? undefined : catalog.get(id.slice(5));
    const label = builtin ? builtin.toUpperCase() : node ? `${node.name} ${node.source_name}` : t('channels.missingNode');
    if (!matches(builtin ? `${label} ${t(`channels.${builtin}`)}` : `${label} ${node?.type ?? id}`)) return [];
    return [{
      id, label, node, builtin, selected: selectedIDs.has(id), disabled: busy,
      unavailable: builtin
        ? (format === 'sing-box' && builtin === 'reject') || (format === 'loon' && group.type !== 'select')
        : !node || node.hidden || !node.available,
    }];
  });
  function updateCandidates(node_ids: string[], builtin_nodes = builtins, candidate_order = order) {
    // Keep the current candidate when possible; an empty group must not leak to direct.
    const default_exit: ChannelRouteExit = group.default_exit.kind === 'node' && node_ids.includes(group.default_exit.id!)
      ? group.default_exit
      : group.default_exit.kind === 'direct' && builtin_nodes.includes('direct')
        ? group.default_exit
        : node_ids.length
          ? { kind: 'node', id: node_ids[0] }
          : { kind: builtin_nodes[0] ?? 'reject' };
    onChange({
      ...group,
      node_ids,
      builtin_nodes,
      candidate_order: candidateOrder({ node_ids, builtin_nodes, candidate_order }),
      default_exit,
      rules: group.rules.map((item) => item.exit.kind === 'node' && !node_ids.includes(item.exit.id!)
        ? { ...item, exit: { kind: 'group-default' } }
        : item),
    });
  }

  function exitLabel(exit: ChannelRouteExit) {
    if (exit.kind === 'node') return nodes.find((node) => node.id === exit.id)?.name ?? t('channels.missingNode');
    return t(exit.kind === 'group-default' ? 'channels.follow' : exit.kind === 'reject' ? 'channels.reject' : 'channels.direct');
  }
  function move(index: number, delta: number) {
    const rules = [...group.rules];
    [rules[index], rules[index + delta]] = [rules[index + delta], rules[index]];
    onChange({ ...group, rules });
  }
  function newRule(remote: boolean) {
    setRule({
      id: crypto.randomUUID(),
      enabled: true,
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
              update_interval: format === 'loon' ? undefined : 86400,
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
          format={format}
          onClose={() => setAddingNodes(false)}
          onAdd={(ids, builtinNodes, addedOrder) => {
            updateCandidates(
              [...new Set([...group.node_ids, ...ids])],
              [...new Set([...builtins, ...builtinNodes])],
              [...order, ...addedOrder],
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
              <Empty className='channel-candidates-empty'>
                <EmptyHeader>
                  <EmptyTitle>{t(search ? 'channels.noMatchingNodes' : 'channels.emptyCandidates')}</EmptyTitle>
                  <EmptyDescription>{t(search ? 'channels.searchNodesHint' : 'channels.emptyCandidatesHint')}</EmptyDescription>
                </EmptyHeader>
              </Empty>
            )}
          </TabsContent>
          <TabsContent value='rules' className='channel-group-content'>
            {format === 'loon' && <p className='channel-delivery-hint'>{t('channels.loonRuleOrder')}</p>}
            <div className='channel-rule-rows'>
              {group.rules.map((item, index) => (
                <div className='channel-rule-row' key={item.id}>
                  <input
                    aria-label={`${t('channels.enabled')} ${item.remote?.name ?? item.value}`}
                    type='checkbox'
                    checked={item.enabled}
                    onChange={(event) => onChange({
                      ...group,
                      rules: group.rules.map((value) =>
                        value.id === item.id ? { ...value, enabled: event.target.checked } : value),
                    })}
                  />
                  <div className='channel-rule-label' title={item.remote?.url ?? item.value}>
                    <span>{item.remote?.name ?? item.value}</span>
                    <span className='channel-rule-summary'>
                      <Badge variant='outline'>{t(item.kind === 'remote' ? 'channels.ruleSet' : `channels.${item.kind}`)}</Badge>
                      <ArrowRight aria-hidden='true' />
                      <Badge variant='info'>{exitLabel(item.exit)}</Badge>
                    </span>
                    {item.remote && !ruleFormats(format).includes(item.remote.format) && (
                      <ErrorNotice error={t('channels.formatPending')} />
                    )}
                  </div>
                  <div className='subscription-toolbar-actions'>
                    <Button variant='ghost' size='icon-sm' aria-label={t('channels.moveUp')} title={t('channels.moveUp')} disabled={index === 0} onClick={() => move(index, -1)}><ArrowUp /></Button>
                    <Button variant='ghost' size='icon-sm' aria-label={t('channels.moveDown')} title={t('channels.moveDown')} disabled={index === group.rules.length - 1} onClick={() => move(index, 1)}><ArrowDown /></Button>
                    <Button variant='ghost' size='icon-sm' aria-label={t('channels.editRule')} title={t('channels.editRule')} onClick={() => setRule(item)}><Pencil /></Button>
                    <Button variant='ghost' size='icon-sm' className='channel-delete-action' aria-label={t('channels.deleteRule')} title={t('channels.deleteRule')} onClick={() => onChange({ ...group, rules: group.rules.filter((value) => value.id !== item.id) })}><Trash2 /></Button>
                  </div>
                </div>
              ))}
            </div>
            {!group.rules.length && (
              <Empty>
                <EmptyHeader>
                  <EmptyMedia variant='icon'><Route /></EmptyMedia>
                  <EmptyTitle>{t('channels.matches')}</EmptyTitle>
                  <EmptyDescription>{t('channels.noRules')}</EmptyDescription>
                </EmptyHeader>
              </Empty>
            )}
          </TabsContent>
        </Tabs>
      </fieldset>
      {rule && (
        <ChannelRuleEditor
          rule={rule}
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

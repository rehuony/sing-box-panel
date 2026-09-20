import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ArrowDown, ArrowRight, ArrowUp, Network, Pencil, Plus, Route, Search, Trash2 } from 'lucide-react';

import type {
  ChannelRouteExit,
  ChannelRule,
  ChannelRuleGroup,
  SubscriptionFormat,
  SubscriptionNodeSummary,
} from '@/api/api-client';

import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Field, FieldGroup, FieldLabel } from '@/components/ui/field';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';

import { ruleFormats } from './channel-policy';
import { ChannelRuleEditor } from './channel-rule-editor';
import { ChannelRouteExitSelect } from './channel-route-exit';
import { subscriptionNodeAddress } from './subscription-node-address';

interface Props {
  busy: boolean;
  group: ChannelRuleGroup;
  format: SubscriptionFormat;
  nodes: SubscriptionNodeSummary[];
  onChange: (group: ChannelRuleGroup) => void;
}
export function ChannelGroupEditor({ group, nodes, format, busy, onChange }: Props) {
  const { t } = useTranslation();
  const [rule, setRule] = useState<ChannelRule | null>(null);
  const [error, setError] = useState('');
  const [search, setSearch] = useState('');
  const candidates = nodes.filter((node) => group.node_ids.includes(node.id));
  const visible = nodes.filter((node) => (!node.hidden && node.available) || group.node_ids.includes(node.id));
  const matches = (value: string) => value.toLowerCase().includes(search.trim().toLowerCase());
  const filtered = visible.filter((node) => matches(`${node.name} ${node.source_name} ${node.type}`));
  const missing = group.node_ids.filter((id) => !nodes.some((node) => node.id === id));
  function select(node_ids: string[]) {
    const exits = [group.default_exit, ...group.rules.map((item) => item.exit)];
    if (exits.some((exit) => exit.kind === 'node' && !node_ids.includes(exit.id!))) {
      setError(t('channels.nodeExitInUse'));
      return;
    }
    setError('');
    onChange({ ...group, node_ids });
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
              update_interval: 86400,
            },
          }
        : { value: '' }),
    });
  }
  return (
    <section className='channel-group-editor' aria-label={t('channels.editGroup')}>
      <fieldset className='channel-group-fields' disabled={busy}>
        <FieldGroup className='channel-group-header'>
          <Field>
            <FieldLabel htmlFor='group-name'>{t('channels.groupName')}</FieldLabel>
            <Input
              id='group-name'
              maxLength={128}
              value={group.name}
              onChange={(event) => onChange({ ...group, name: event.target.value })}
            />
          </Field>
          <label className='channel-group-enabled'>
            <input type='checkbox' checked={group.enabled} onChange={(event) => onChange({ ...group, enabled: event.target.checked })} />
            {t('channels.enabled')}
          </label>
        </FieldGroup>
        <Tabs defaultValue='nodes' className='channel-group-tabs'>
          <TabsList className='subscriptions-tabs' aria-label={t('channels.editGroup')}>
            <TabsTrigger value='nodes'>{t('channels.candidates')}</TabsTrigger>
            <TabsTrigger value='rules' onClick={() => setError('')}>{t('channels.matches')}</TabsTrigger>
          </TabsList>
          <TabsContent value='nodes' className='channel-group-content'>
            <div className='channel-members-toolbar'>
              <div className='channel-members-title'>
                <Network aria-hidden='true' />
                <h3>{t('channels.members')}</h3>
                <Badge variant='secondary'>{group.node_ids.length}</Badge>
              </div>
              <div className='channel-selection-actions'>
                <Button size='sm' variant='outline' onClick={() => select([...new Set([
                  ...group.node_ids,
                  ...filtered.filter((node) => !node.hidden && node.available).map((node) => node.id),
                ])])}>
                  {t('channels.selectAll')}
                </Button>
                <Button size='sm' variant='outline' onClick={() => select([])}>{t('channels.selectNone')}</Button>
              </div>
            </div>
            <div className='channel-node-search'>
              <Search aria-hidden='true' />
              <Input aria-label={t('channels.searchNodes')} placeholder={t('channels.searchNodes')} value={search} onChange={(event) => setSearch(event.target.value)} />
            </div>
            {error && <p role='alert' className='subscription-form-error'>{error}</p>}
            <div className='channel-candidates'>
              {filtered.map((node) => (
                <label className='channel-node-option' key={node.id} data-selected={group.node_ids.includes(node.id)}>
                  <input
                    type='checkbox'
                    aria-label={`${node.name} ${node.source_name}`}
                    checked={group.node_ids.includes(node.id)}
                    onChange={(event) => select(event.target.checked
                      ? [...group.node_ids, node.id]
                      : group.node_ids.filter((id) => id !== node.id))}
                  />
                  <span className='channel-node-option-name' title={node.name}>{node.name}</span>
                  <span className='channel-node-badges'>
                    <Badge variant='info'>{node.type}</Badge>
                    <Badge variant='outline' title={node.source_name}>{node.source_name}</Badge>
                    {node.tls && <Badge variant='success'>{node.reality ? 'Reality' : 'TLS'}</Badge>}
                    {(node.hidden || !node.available) && <Badge variant='warning'>{t('channels.unavailable')}</Badge>}
                  </span>
                  <span className='channel-node-address' title={subscriptionNodeAddress(node)}>{subscriptionNodeAddress(node) || t('subscriptions.nodes.hostMissing')}</span>
                </label>
              ))}
              {missing.filter(matches).map((id) => (
                <label className='channel-node-option' data-selected key={id}>
                  <input type='checkbox' checked onChange={() => select(group.node_ids.filter((value) => value !== id))} />
                  <span className='channel-node-option-name' title={id}>{t('channels.missingNode')}</span>
                  <Badge variant='warning'>{t('channels.unavailable')}</Badge>
                </label>
              ))}
            </div>
            {!filtered.length && !missing.some(matches) && (
              <Empty>
                <EmptyHeader>
                  <EmptyMedia variant='icon'><Search /></EmptyMedia>
                  <EmptyTitle>{t(search ? 'channels.noMatchingNodes' : 'channels.noNodes')}</EmptyTitle>
                  {search && <EmptyDescription>{t('channels.searchNodesHint')}</EmptyDescription>}
                </EmptyHeader>
              </Empty>
            )}
          </TabsContent>
          <TabsContent value='rules' className='channel-group-content'>
            <div className='subscription-settings-fields channel-group-exit'>
              <label htmlFor='group-default'>{t('channels.defaultExit')}</label>
              <ChannelRouteExitSelect id='group-default' value={group.default_exit} nodes={candidates} onChange={(default_exit) => onChange({ ...group, default_exit })} />
            </div>
            <div className='channel-selection-actions'>
              <Button size='sm' variant='outline' onClick={() => newRule(false)}>
                <Plus data-icon='inline-start' />
                {t('channels.addRule')}
              </Button>
              <Button size='sm' variant='outline' onClick={() => newRule(true)}>
                <Plus data-icon='inline-start' />
                {t('channels.addRemote')}
              </Button>
            </div>
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
                      <small className='subscription-form-error'>{t('channels.formatPending')}</small>
                    )}
                  </div>
                  <div className='subscription-toolbar-actions'>
                    <Button variant='outline' size='icon-sm' aria-label={t('channels.moveUp')} disabled={index === 0} onClick={() => move(index, -1)}><ArrowUp /></Button>
                    <Button variant='outline' size='icon-sm' aria-label={t('channels.moveDown')} disabled={index === group.rules.length - 1} onClick={() => move(index, 1)}><ArrowDown /></Button>
                    <Button variant='outline' size='icon-sm' aria-label={t('channels.editRule')} onClick={() => setRule(item)}><Pencil /></Button>
                    <Button variant='destructive' size='icon-sm' aria-label={t('channels.deleteRule')} onClick={() => onChange({ ...group, rules: group.rules.filter((value) => value.id !== item.id) })}><Trash2 /></Button>
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
          nodes={candidates}
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

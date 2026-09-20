import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ArrowDown, ArrowUp, Layers3, Plus, RefreshCw, Save, Search, Trash2 } from 'lucide-react';

import type {
  ChannelNativeTemplate,
  ChannelPolicy,
  ChannelRuleGroup,
  SubscriptionChannel,
  SubscriptionChannelConfig,
  SubscriptionFormat,
  SubscriptionNodeSummary,
  SubscriptionPreview,
} from '@/api/api-client';

import { Badge } from '@/components/ui/badge';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { Field, FieldLabel } from '@/components/ui/field';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { describeRequestError } from '@/components/error-notice';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from '@/components/ui/empty';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';

import { ChannelOptions } from './channel-options';
import { ChannelPreview } from './channel-preview';
import { ChannelGroupEditor } from './channel-group-editor';
import { ChannelRouteExitSelect } from './channel-route-exit';
import { SubscriptionNodeGrid } from './subscription-node-grid';
import { ChannelTemplateEditor } from './channel-template-editor';
import { SubscriptionNodeEditor } from './subscription-node-editor';
import {
  incompatiblePolicy,
  initialChannelPolicy,
  newRuleGroup,
  selectedNodeIDs,
  toggleChannelNode,
} from './channel-policy';

interface Props {
  active?: boolean;
  onBack: () => void;
  channel: SubscriptionChannel;
  onRefresh: () => Promise<void>;
  nodes: SubscriptionNodeSummary[];
  toolbarTarget?: HTMLElement | null;
  onSaved: (channel: SubscriptionChannel) => void;
}
export function ChannelWorkspace({ active = true, toolbarTarget, channel, nodes, onBack, onSaved, onRefresh }: Props) {
  const { t } = useTranslation();
  const client = useApiClient();
  const [policy, setPolicy] = useState(() => initialChannelPolicy(channel, nodes));
  const [config, setConfig] = useState(channel.config);
  const [format, setFormat] = useState<SubscriptionFormat>(channel.format);
  const [name, setName] = useState(channel.name);
  const [tab, setTab] = useState<'nodes' | 'rules'>(channel.format === 'loon' ? 'nodes' : 'rules');
  const [search, setSearch] = useState('');
  const [busy, setBusy] = useState(false);
  const [options, setOptions] = useState<'organizer' | 'distribution' | null>(null);
  const [templateOpen, setTemplateOpen] = useState(false);
  const [groupID, setGroupID] = useState<string | null>(channel.config.policy?.groups[0]?.id ?? null);
  const [removeGroup, setRemoveGroup] = useState<string | null>(null);
  const [node, setNode] = useState<SubscriptionNodeSummary | null>(null);
  const [preview, setPreview] = useState<SubscriptionPreview | null>(null);
  const [leaving, setLeaving] = useState(false);
  const lifeRef = useRef<AbortController | null>(null);
  const signature = (
    p: ChannelPolicy,
    f: SubscriptionFormat,
    c: SubscriptionChannelConfig,
    n: string,
  ) => JSON.stringify({ p, f, c: { ...c, policy: undefined }, n });
  const [savedSignature, setSavedSignature] = useState(() =>
    signature(policy, format, config, name),
  );
  const dirty = signature(policy, format, config, name) !== savedSignature;
  useEffect(() => {
    const controller = new AbortController();
    lifeRef.current = controller;
    return () => controller.abort();
  }, []);
  useEffect(() => {
    if (!dirty) return;
    const prevent = (event: BeforeUnloadEvent) => {
      event.preventDefault();
    };
    window.addEventListener('beforeunload', prevent);
    return () => window.removeEventListener('beforeunload', prevent);
  }, [dirty]);
  const selected = useMemo(() => selectedNodeIDs(policy, nodes), [policy, nodes]);
  const candidates = nodes.filter((value) => selected.has(value.id));
  const visibleCandidates = candidates.filter((value) => !value.hidden && value.available);
  const group = policy.groups.find((value) => value.id === groupID) ?? policy.groups[0];
  const groupIndex = group ? policy.groups.indexOf(group) : -1;
  const conflict = format !== 'loon' && incompatiblePolicy(policy, format);
  function policyConfig(p: ChannelPolicy) {
    return { ...config, ...(format === 'loon' ? {} : { policy: p }) };
  }
  async function render(p: ChannelPolicy = policy) {
    return client.previewSubscriptionChannel(channel.id, '', lifeRef.current?.signal, {
      format,
      config: policyConfig(p),
    });
  }
  async function persist(p: ChannelPolicy = policy) {
    if (!name.trim() || (format !== 'loon' && p.groups.some((value) => !value.name.trim()))) throw new Error(t('channels.nameRequired'));
    p = { ...p, groups: p.groups.map((value) => ({ ...value, name: value.name.trim() })) };
    const result = await client.updateSubscriptionChannel(
      channel.id,
      {
        name: name.trim(),
        enabled: channel.enabled,
        format,
        public_host: channel.public_host,
        config: policyConfig(p),
      },
      channel.updated_at,
      lifeRef.current?.signal,
    );
    if (lifeRef.current?.signal.aborted) return;
    setPolicy(p);
    setConfig(result.config);
    onSaved(result);
    setSavedSignature(signature(p, format, result.config, name.trim()));
    setName(name.trim());
    toast.add({ title: t('channels.saved'), type: 'success' });
  }
  async function save() {
    if (busy) return;
    setBusy(true);
    try {
      await persist();
    } catch (reason) {
      if (!lifeRef.current?.signal.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      if (!lifeRef.current?.signal.aborted) setBusy(false);
    }
  }
  async function showPreview() {
    setBusy(true);
    try {
      const value = await render();
      if (!lifeRef.current?.signal.aborted) setPreview(value);
    } catch (reason) {
      if (!lifeRef.current?.signal.aborted) toast.add({ title: describeRequestError(reason), type: 'error' });
    } finally {
      if (!lifeRef.current?.signal.aborted) setBusy(false);
    }
  }
  function toggle(id: string) {
    if (
      selected.has(id)
      && (policy.groups.some((value) => value.node_ids.includes(id))
        || (policy.default_exit.kind === 'node' && policy.default_exit.id === id))
    ) {
      toast.add({ title: t('channels.selectedByGroup'), type: 'error' });
      return;
    }
    setPolicy((current) => toggleChannelNode(current, id, !selected.has(id)));
  }
  function reorder(index: number, delta: number) {
    const groups = [...policy.groups];
    [groups[index], groups[index + delta]] = [groups[index + delta], groups[index]];
    setPolicy({ ...policy, groups });
  }
  function updateGroup(value: ChannelRuleGroup) {
    if (!value.enabled && policy.default_exit.kind === 'group' && policy.default_exit.id === value.id) {
      toast.add({ title: t('channels.groupExitInUse'), type: 'error' });
      return;
    }
    setPolicy((current) => ({
      ...current,
      selection: {
        ...current.selection,
        ids: [...new Set([...current.selection.ids, ...value.node_ids])],
        excluded_ids: current.selection.excluded_ids.filter((id) => !value.node_ids.includes(id)),
      },
      groups: current.groups.map((item) => item.id === value.id ? value : item),
    }));
  }
  function addGroup() {
    // Fixed snapshot: subsequent catalog changes never modify an existing group.
    const ids = visibleCandidates.map((value) => value.id);
    let number = policy.groups.length + 1;
    let name = t('channels.newGroupName', { number });
    while (policy.groups.some((value) => value.name === name)) name = t('channels.newGroupName', { number: ++number });
    const value = { ...newRuleGroup(ids), name };
    setPolicy((current) => ({
      ...current,
      selection: { ...current.selection, ids: [...new Set([...current.selection.ids, ...ids])] },
      groups: [...current.groups, value],
    }));
    setGroupID(value.id);
  }
  return (
    <Tabs className='channel-workspace' value={tab} onValueChange={(value) => setTab(value as 'nodes' | 'rules')}>
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='subscription-source-toolbar channel-detail-toolbar workspace-toolbar-content'>
          <div className='subscription-detail-tabs'>
            <Button disabled={busy} variant='outline' onClick={() => (dirty ? setLeaving(true) : onBack())}>
              {t('channels.back')}
            </Button>
            <TabsList className='subscriptions-tabs' aria-label={t('channels.title')}>
              <TabsTrigger value='nodes' disabled={busy}>{t('channels.nodes')}</TabsTrigger>
              <TabsTrigger value='rules' disabled={busy}>{t('channels.rules')}</TabsTrigger>
            </TabsList>
          </div>
          <div className='channel-save'>
            <Button disabled={busy || conflict || !dirty} variant='default' onClick={() => void save()}>
              <Save data-icon='inline-start' />
              {t('channels.save')}
              {dirty && <span aria-label={t('channels.unsaved')}> ·</span>}
            </Button>
          </div>
          {tab === 'nodes' && (
            <div className='subscription-search'>
              <Search />
              <input aria-label={t('subscriptions.sources.search')} placeholder={t('subscriptions.sources.search')} value={search} onChange={(event) => setSearch(event.target.value)} />
            </div>
          )}
        </div>
      </ToolbarActions>
      <TabsContent value='nodes' className='subscription-node-list'>
        <div className='channel-node-tools'>
          <Button variant='outline' disabled={busy || format === 'loon'} onClick={() => setOptions('organizer')}>{t('channels.organizer')}</Button>
          <Button size='icon' variant='outline' aria-label={t('channels.refresh')} disabled={busy} onClick={() => void onRefresh()}><RefreshCw /></Button>
        </div>
        <SubscriptionNodeGrid nodes={nodes} readOnly={format === 'loon'} search={search} selected={selected} onSelect={toggle} onOpen={setNode} busy={busy} />
      </TabsContent>
      <TabsContent value='rules' className='channel-rules-workspace'>
        <fieldset className='channel-rules-toolbar' disabled={busy}>
          <Field orientation='horizontal' className='channel-name-field'>
            <FieldLabel htmlFor='channel-edit-name'>{t('channels.name')}</FieldLabel>
            <Input id='channel-edit-name' value={name} onChange={(event) => setName(event.target.value)} />
          </Field>
          <label htmlFor='channel-output'>{t('channels.client')}</label>
          <select id='channel-output' value={format} onChange={(event) => setFormat(event.target.value as SubscriptionFormat)}>
            <option value='sing-box'>sing-box JSON</option>
            <option value='mihomo'>Mihomo YAML</option>
            {channel.format === 'loon' && <option value='loon'>Loon</option>}
          </select>
          <Button variant='outline' onClick={() => setOptions('distribution')}>{t('channels.distribution')}</Button>
          <Button variant='outline' onClick={() => void showPreview()}>{t('channels.preview')}</Button>
        </fieldset>
        {conflict && <p className='subscription-form-error' role='status'>{t('channels.formatConflict')}</p>}
        {format === 'loon'
          ? <p>{t('channels.legacy')}</p>
          : (
              <>
                <div className='channel-policy-layout'>
                  <aside className='channel-group-sidebar' aria-label={t('channels.groups')}>
                    <div className='channel-group-sidebar-heading'>
                      <h2>{t('channels.groups')}</h2>
                      <Badge variant='secondary'>{policy.groups.length}</Badge>
                      <Button size='sm' variant='outline' disabled={busy} onClick={addGroup}>
                        <Plus data-icon='inline-start' />
                        {t('channels.addGroup')}
                      </Button>
                    </div>
                    <div className='channel-group-list'>
                      {policy.groups.map((value) => (
                        <Button
                          key={value.id}
                          variant='outline'
                          className='channel-group-item'
                          size='content'
                          aria-pressed={group?.id === value.id}
                          disabled={busy}
                          onClick={() => setGroupID(value.id)}
                        >
                          <span className='channel-group-item-name' title={value.name}>
                            <Layers3 aria-hidden='true' />
                            {value.name || t('channels.groupName')}
                          </span>
                          <span className='channel-group-badges'>
                            <Badge variant='info'>{t('channels.membersCount', { count: value.node_ids.length })}</Badge>
                            <Badge variant='outline'>{t('channels.manualCount', { count: value.rules.length })}</Badge>
                            {!value.enabled && <Badge variant='secondary'>{t('channels.disabled')}</Badge>}
                            {policy.default_exit.kind === 'group' && policy.default_exit.id === value.id && <Badge variant='success'>{t('channels.finalExit')}</Badge>}
                          </span>
                        </Button>
                      ))}
                    </div>
                    {group && (
                      <div className='channel-group-actions'>
                        <Button size='icon-sm' variant='outline' aria-label={t('channels.moveGroupUp')} disabled={busy || groupIndex === 0} onClick={() => reorder(groupIndex, -1)}><ArrowUp /></Button>
                        <Button size='icon-sm' variant='outline' aria-label={t('channels.moveGroupDown')} disabled={busy || groupIndex === policy.groups.length - 1} onClick={() => reorder(groupIndex, 1)}><ArrowDown /></Button>
                        <Button variant='destructive' size='sm' disabled={busy} onClick={() => {
                          if (policy.default_exit.kind === 'group' && policy.default_exit.id === group.id) {
                            toast.add({ title: t('channels.groupExitInUse'), type: 'error' });
                            return;
                          }
                          setRemoveGroup(group.id);
                        }}>
                          <Trash2 data-icon='inline-start' />
                          {t('channels.deleteGroup')}
                        </Button>
                      </div>
                    )}
                    <fieldset disabled={busy} className='channel-fallback'>
                      <label htmlFor='channel-fallback'>{t('channels.fallback')}</label>
                      <ChannelRouteExitSelect
                        id='channel-fallback'
                        value={policy.default_exit}
                        nodes={candidates}
                        groups={policy.groups}
                        onChange={(default_exit) => setPolicy({
                          ...policy,
                          default_exit,
                          selection: {
                            ...policy.selection,
                            ids: default_exit.kind === 'node' ? [...new Set([...policy.selection.ids, default_exit.id!])] : policy.selection.ids,
                          },
                        })}
                      />
                    </fieldset>
                  </aside>
                  {group
                    ? (
                        <ChannelGroupEditor
                          key={group.id} group={group} nodes={nodes}
                          format={format} busy={busy} onChange={updateGroup}
                        />
                      )
                    : (
                        <Empty className='channel-group-empty'>
                          <EmptyHeader>
                            <EmptyMedia variant='icon'><Layers3 /></EmptyMedia>
                            <EmptyTitle>{t('channels.noGroups')}</EmptyTitle>
                            <EmptyDescription>{t('channels.noGroupsHint')}</EmptyDescription>
                          </EmptyHeader>
                          <EmptyContent>
                            <Button disabled={busy} onClick={addGroup}>
                              <Plus data-icon='inline-start' />
                              {t('channels.createFirstGroup')}
                            </Button>
                          </EmptyContent>
                        </Empty>
                      )}
                </div>

              </>
            )}
      </TabsContent>
      {options && (
        <ChannelOptions
          kind={options}
          channelID={channel.id}
          needsSave={dirty}
          enabled={channel.enabled}
          legacy={format === 'loon'}
          policy={policy}
          config={config}
          onClose={() => setOptions(null)}
          onSave={(p, c) => {
            setPolicy((current) =>
              options === 'organizer'
                ? { ...current, organizer: p.organizer }
                : {
                    ...current,
                    selection: {
                      ...current.selection,
                      new_node_policy: p.selection.new_node_policy,
                    },
                  },
            );
            setConfig((current) => ({
              ...current,
              exclude_tags: c.exclude_tags,
              exclude_types: c.exclude_types,
            }));
          }}
          onTemplate={() => setTemplateOpen(true)}
        />
      )}
      {templateOpen && format !== 'loon' && (
        <ChannelTemplateEditor
          template={policy.template}
          format={format}
          onClose={() => setTemplateOpen(false)}
          onPreview={(template: ChannelNativeTemplate) => render({ ...policy, template })}
          onSave={(template) => persist({ ...policy, template })}
        />
      )}
      {preview && <ChannelPreview preview={preview} onClose={() => setPreview(null)} />}
      {node && (
        <SubscriptionNodeEditor
          node={node}
          onClose={() => setNode(null)}
          onSaved={() => void onRefresh()}
        />
      )}
      <Dialog open={leaving} onOpenChange={setLeaving}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('channels.leave')}</DialogTitle>
            <DialogDescription className='sr-only'>{t('channels.unsaved')}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant='outline' onClick={() => setLeaving(false)}>
              {t('common.cancel')}
            </Button>
            <Button variant='outline' onClick={onBack}>
              {t('channels.discard')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={removeGroup != null} onOpenChange={(open) => !open && setRemoveGroup(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('channels.deleteGroup')}</DialogTitle>
            <DialogDescription>
              {policy.groups.find((value) => value.id === removeGroup)?.name}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant='outline' onClick={() => setRemoveGroup(null)}>
              {t('common.cancel')}
            </Button>
            <Button
              variant='destructive'
              onClick={() => {
                setPolicy({
                  ...policy,
                  groups: policy.groups.filter((value) => value.id !== removeGroup),
                });
                setRemoveGroup(null);
              }}
            >
              {t('channels.deleteGroup')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Tabs>
  );
}

import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ArrowDown, ArrowUp, Pencil, Plus, RefreshCw, Search, Trash2 } from 'lucide-react';

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

import { Button } from '@/components/ui/button';
import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { ToolbarActions } from '@/components/workspace-toolbar';
import { describeRequestError } from '@/components/error-notice';
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
  const [enabled, setEnabled] = useState(channel.enabled);
  const [tab, setTab] = useState<'nodes' | 'rules'>('nodes');
  const [search, setSearch] = useState('');
  const [busy, setBusy] = useState(false);
  const [options, setOptions] = useState<'organizer' | 'distribution' | null>(null);
  const [templateOpen, setTemplateOpen] = useState(false);
  const [group, setGroup] = useState<ChannelRuleGroup | null>(null);
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
    e: boolean,
  ) => JSON.stringify({ p, f, c: { ...c, policy: undefined }, n, e });
  const [savedSignature, setSavedSignature] = useState(() =>
    signature(policy, format, config, name, enabled),
  );
  const dirty = signature(policy, format, config, name, enabled) !== savedSignature;
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
    if (!name.trim()) throw new Error(t('channels.nameRequired'));
    const result = await client.updateSubscriptionChannel(
      channel.id,
      {
        name: name.trim(),
        enabled,
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
    setSavedSignature(signature(p, format, result.config, name.trim(), enabled));
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
  function addGroup() {
    // Fixed snapshot: subsequent catalog changes never modify an existing group.
    const ids = visibleCandidates.map((value) => value.id);
    setPolicy((current) => ({
      ...current,
      selection: { ...current.selection, ids: [...new Set([...current.selection.ids, ...ids])] },
    }));
    setGroup(newRuleGroup(ids));
  }
  return (
    <>
      <ToolbarActions active={active} target={toolbarTarget}>
        <div className='subscription-source-toolbar channel-detail-toolbar workspace-toolbar-content'>
          <div className='subscription-detail-tabs'>
            <Button
              disabled={busy}
              variant='ghost'
              onClick={() => (dirty ? setLeaving(true) : onBack())}
            >
              {t('channels.back')}
            </Button>
            <div role='tablist' aria-label={t('channels.title')}>
              <Button
                role='tab'
                aria-selected={tab === 'nodes'}
                variant={tab === 'nodes' ? 'secondary' : 'ghost'}
                onClick={() => setTab('nodes')}
              >
                {t('channels.nodes')}
              </Button>
              <Button
                role='tab'
                aria-selected={tab === 'rules'}
                variant={tab === 'rules' ? 'secondary' : 'ghost'}
                onClick={() => setTab('rules')}
              >
                {t('channels.rules')}
              </Button>
            </div>
          </div>
          <div className='channel-save'>
            <Button
              disabled={busy || conflict || !dirty}
              variant='secondary'
              onClick={() => void save()}
            >
              {t('channels.save')}
              {dirty && <span aria-label={t('channels.unsaved')}> ·</span>}
            </Button>
          </div>
          {tab === 'nodes' && (
            <div className='subscription-search'>
              <Search />
              <input
                aria-label={t('subscriptions.sources.search')}
                placeholder={t('subscriptions.sources.search')}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
              />
            </div>
          )}
        </div>
      </ToolbarActions>
      {tab === 'nodes'
        ? (
            <>
              <div className='channel-node-tools'>
                <Button
                  variant='ghost'
                  disabled={format === 'loon'}
                  onClick={() => setOptions('organizer')}
                >
                  {t('channels.organizer')}
                </Button>
                <div className='subscription-toolbar-actions'>
                  <Button
                    size='icon'
                    variant='ghost'
                    aria-label={t('channels.refresh')}
                    disabled={busy}
                    onClick={() => void onRefresh()}
                  >
                    <RefreshCw />
                  </Button>
                </div>
              </div>
              <SubscriptionNodeGrid
                nodes={nodes}
                readOnly={format === 'loon'}
                search={search}
                selected={selected}
                onSelect={toggle}
                onOpen={setNode}
                busy={busy}
              />
            </>
          )
        : (
            <div className='channel-rules-workspace'>
              <div className='channel-rules-toolbar'>
                <label htmlFor='channel-output'>{t('channels.client')}</label>
                <select
                  id='channel-output'
                  value={format}
                  onChange={(event) => setFormat(event.target.value as SubscriptionFormat)}
                >
                  <option value='sing-box'>sing-box JSON</option>
                  <option value='mihomo'>Mihomo YAML</option>
                  {channel.format === 'loon' && <option value='loon'>Loon</option>}
                </select>
                <div />
                <Button
                  variant='ghost'
                  disabled={format === 'loon'}
                  onClick={() => setOptions('distribution')}
                >
                  {t('channels.distribution')}
                </Button>
                <Button variant='secondary' disabled={busy} onClick={() => void showPreview()}>
                  {t('channels.preview')}
                </Button>
              </div>
              {conflict && (
                <p className='subscription-form-error' role='status'>
                  {t('channels.formatConflict')}
                </p>
              )}
              {format === 'loon'
                ? (
                    <p>{t('channels.legacy')}</p>
                  )
                : (
                    <>
                      <div className='channel-group-title'>
                        <span>{t('channels.groups')}</span>
                        <div className='subscription-toolbar-actions'>
                          <Button
                            variant='ghost'
                            size='icon'
                            aria-label={t('channels.addGroup')}
                            onClick={addGroup}
                          >
                            <Plus />
                          </Button>
                        </div>
                      </div>
                      <div className='channel-groups-scroll'>
                        <table className='workspace-table channel-groups-table'>
                          <thead>
                            <tr>
                              <th>{t('channels.groupName')}</th>
                              <th>{t('channels.matches')}</th>
                              <th>{t('channels.state')}</th>
                              <th>{t('channels.actions')}</th>
                            </tr>
                          </thead>
                          <tbody>
                            {policy.groups.map((value, index) => (
                              <tr key={value.id}>
                                <td>
                                  <button title={value.name} onClick={() => setGroup(value)}>
                                    {value.name}
                                  </button>
                                </td>
                                <td>
                                  <div className='channel-counts'>
                                    <span
                                      className={
                                        value.rules.some((rule) => rule.kind !== 'remote') ? '' : 'is-zero'
                                      }
                                    >
                                      {t('channels.manualCount', {
                                        count: value.rules.filter((rule) => rule.kind !== 'remote').length,
                                      })}
                                    </span>
                                    <span
                                      className={
                                        value.rules.some((rule) => rule.kind === 'remote') ? '' : 'is-zero'
                                      }
                                    >
                                      {t('channels.remoteCount', {
                                        count: value.rules.filter((rule) => rule.kind === 'remote').length,
                                      })}
                                    </span>
                                  </div>
                                </td>
                                <td>
                                  <input
                                    type='checkbox'
                                    aria-label={`${t('channels.enabled')} ${value.name}`}
                                    checked={value.enabled}
                                    onChange={(event) =>
                                      setPolicy({
                                        ...policy,
                                        groups: policy.groups.map((item) =>
                                          item.id === value.id
                                            ? { ...item, enabled: event.target.checked }
                                            : item,
                                        ),
                                      })
                                    }
                                  />
                                </td>
                                <td>
                                  <div className='subscription-toolbar-actions'>
                                    <Button
                                      size='icon'
                                      variant='ghost'
                                      aria-label={t('channels.moveUp')}
                                      disabled={index === 0}
                                      onClick={() => reorder(index, -1)}
                                    >
                                      <ArrowUp />
                                    </Button>
                                    <Button
                                      size='icon'
                                      variant='ghost'
                                      aria-label={t('channels.moveDown')}
                                      disabled={index === policy.groups.length - 1}
                                      onClick={() => reorder(index, 1)}
                                    >
                                      <ArrowDown />
                                    </Button>
                                    <Button
                                      size='icon'
                                      variant='ghost'
                                      aria-label={t('channels.editGroup')}
                                      onClick={() => setGroup(value)}
                                    >
                                      <Pencil />
                                    </Button>
                                    <Button
                                      size='icon'
                                      variant='ghost'
                                      aria-label={t('channels.deleteGroup')}
                                      onClick={() => setRemoveGroup(value.id)}
                                    >
                                      <Trash2 />
                                    </Button>
                                  </div>
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                      <div className='subscription-settings-fields channel-fallback'>
                        <label htmlFor='channel-fallback'>{t('channels.fallback')}</label>
                        <ChannelRouteExitSelect
                          id='channel-fallback'
                          value={policy.default_exit}
                          nodes={candidates}
                          groups={policy.groups}
                          onChange={(default_exit) =>
                            setPolicy({
                              ...policy,
                              default_exit,
                              selection: {
                                ...policy.selection,
                                ids:
                          default_exit.kind === 'node'
                            ? [...new Set([...policy.selection.ids, default_exit.id!])]
                            : policy.selection.ids,
                              },
                            })
                          }
                        />
                      </div>
                    </>
                  )}
              <details className='channel-metadata'>
                <summary>{t('channels.name')}</summary>
                <div className='subscription-settings-fields'>
                  <label htmlFor='channel-edit-name'>{t('channels.name')}</label>
                  <input
                    id='channel-edit-name'
                    value={name}
                    onChange={(event) => setName(event.target.value)}
                  />
                  <label htmlFor='channel-enabled'>{t('channels.enabledLabel')}</label>
                  <input
                    id='channel-enabled'
                    checked={enabled}
                    type='checkbox'
                    onChange={(event) => setEnabled(event.target.checked)}
                  />
                </div>
              </details>
            </div>
          )}
      {group && (
        <ChannelGroupEditor
          group={group}
          nodes={candidates}
          format={format}
          onClose={() => setGroup(null)}
          onSave={async (value) => {
            setPolicy((current) => ({
              ...current,
              groups: current.groups.some((item) => item.id === value.id)
                ? current.groups.map((item) => (item.id === value.id ? value : item))
                : [...current.groups, value],
            }));
          }}
        />
      )}
      {options && (
        <ChannelOptions
          kind={options}
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
            <Button variant='secondary' onClick={() => setLeaving(false)}>
              {t('common.cancel')}
            </Button>
            <Button variant='secondary' onClick={onBack}>
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
            <Button variant='secondary' onClick={() => setRemoveGroup(null)}>
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
    </>
  );
}

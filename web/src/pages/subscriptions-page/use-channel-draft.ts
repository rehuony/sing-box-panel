import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';

import type { ChannelPolicy, ChannelRuleGroup, SubscriptionChannel, SubscriptionChannelConfig, SubscriptionFormat, SubscriptionNodeSummary, SubscriptionPreview } from '@/api/api-client';

import { toast } from '@/components/ui/toast-manager';
import { useApiClient } from '@/api/api-client-context';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';
import { channelTemplateDefaults } from '@/constants/channel-templates';

import { incompatiblePolicy, initialChannelPolicy, newRuleGroup } from './channel-policy';

export function useChannelDraft(
  channel: SubscriptionChannel,
  nodes: SubscriptionNodeSummary[],
  onSaved: (channel: SubscriptionChannel) => void,
) {
  const { t } = useTranslation();
  const client = useApiClient();
  const [initialPolicy] = useState(() => initialChannelPolicy(channel, nodes));
  const [policy, setPolicy] = useState(initialPolicy);
  const [config, setConfig] = useState(channel.config);
  const [format, setFormat] = useState<SubscriptionFormat>(channel.format);
  const [name, setName] = useState(channel.name);
  const [busy, setBusy] = useState(false);
  const [optionsOpen, setOptionsOpen] = useState(false);
  const [templateOpen, setTemplateOpen] = useState(false);
  const [groupID, setGroupID] = useState<string | null>(channel.config.policy?.groups[0]?.id ?? null);
  const [removeGroup, setRemoveGroup] = useState<string | null>(null);
  const [groupSettings, setGroupSettings] = useState<ChannelRuleGroup | null>(null);
  const [preview, setPreview] = useState<SubscriptionPreview | null>(null);
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
  const settingsDirty = groupSettings !== null
    && JSON.stringify(groupSettings) !== JSON.stringify(policy.groups.find(value => value.id === groupSettings.id));
  const confirmNavigation = useUnsavedChanges(dirty || settingsDirty, () => {
    const restoredPolicy = structuredClone(channel.config.policy ?? initialPolicy);
    setPolicy(restoredPolicy);
    setConfig(channel.config);
    setFormat(channel.format);
    setName(channel.name);
    setSavedSignature(signature(restoredPolicy, channel.format, channel.config, channel.name));
    setGroupSettings(null);
    setOptionsOpen(false);
    setTemplateOpen(false);
  }, busy);
  useEffect(() => {
    const controller = new AbortController();
    lifeRef.current = controller;
    return () => controller.abort();
  }, []);
  const group = policy.groups.find((value) => value.id === groupID) ?? policy.groups[0];
  const groupIndex = group ? policy.groups.indexOf(group) : -1;
  const finalGroup = policy.default_exit.kind === 'group' && policy.default_exit.id === group?.id;
  const conflict = incompatiblePolicy(policy, format);
  function policyConfig(p: ChannelPolicy) {
    // Metadata-only edits must not upgrade persisted legacy channels.
    const unchangedLegacy = !channel.config.policy && format === channel.format
      && JSON.stringify(p) === JSON.stringify(initialPolicy);
    return { ...config, policy: unchangedLegacy ? undefined : p };
  }
  async function render(p: ChannelPolicy = policy, signal = lifeRef.current?.signal) {
    return client.previewSubscriptionChannel(channel.id, '', signal, {
      format,
      config: policyConfig(p),
    });
  }
  async function persist(p: ChannelPolicy = policy) {
    if (!name.trim() || p.groups.some((value) => !value.name.trim())) throw new Error(t('channels.nameRequired'));
    p = { ...p, groups: p.groups.map((value) => ({ ...value, name: value.name.trim() })) };
    if (client.supportsNativeChannelValidation) await render(p);
    if (lifeRef.current?.signal.aborted) return;
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
    let number = policy.groups.length + 1;
    let name = t('channels.newGroupName', { number });
    while (policy.groups.some((value) => value.name === name)) name = t('channels.newGroupName', { number: ++number });
    const value = { ...newRuleGroup([]), name };
    setPolicy((current) => ({
      ...current,
      groups: [...current.groups, value],
    }));
    setGroupID(value.id);
  }
  function applyOptions(
    p: ChannelPolicy,
    c: SubscriptionChannelConfig,
    identity: { name: string; format: SubscriptionFormat },
  ) {
    setName(identity.name);
    setFormat(identity.format);
    setPolicy((current) => ({
      ...current,
      template: identity.format !== format && current.template
        ? { format: identity.format, content: channelTemplateDefaults[identity.format] }
        : current.template,
      organizer: p.organizer,
      selection: {
        ...current.selection,
        new_node_policy: p.selection.new_node_policy,
      },
    }));
    setConfig((current) => ({
      ...current,
      exclude_tags: c.exclude_tags,
      exclude_types: c.exclude_types,
    }));
  }
  return {
    policy,
    setPolicy,
    config,
    format,
    name,
    busy,
    optionsOpen,
    setOptionsOpen,
    templateOpen,
    setTemplateOpen,
    group,
    groupIndex,
    finalGroup,
    groupSettings,
    setGroupSettings,
    setGroupID,
    removeGroup,
    setRemoveGroup,
    preview,
    setPreview,
    dirty,
    conflict,
    confirmNavigation,
    render,
    save,
    showPreview,
    reorder,
    updateGroup,
    addGroup,
    applyOptions,
  };
}

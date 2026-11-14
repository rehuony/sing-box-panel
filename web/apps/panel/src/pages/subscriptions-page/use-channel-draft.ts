import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';
import { toast } from '@shared/components/ui/toast-manager';

import type { ChannelPolicy, ChannelRule, ChannelRuleGroup, SubscriptionChannel, SubscriptionChannelConfig, SubscriptionFormat, SubscriptionNodeSummary, SubscriptionPreview } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';
import { channelTemplateDefaults } from '@/constants/channel-templates';

import type { RuleTextResult } from './channel-rule-text';

import { parseRules, serializeRules } from './channel-rule-text';
import { incompatiblePolicy, initialChannelPolicy, newRuleGroup, nextRuleIndex, removeGroupReferences } from './channel-policy';

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
  const [previewError, setPreviewError] = useState<unknown>(null);
  const [preview, setPreview] = useState<SubscriptionPreview | null>(null);
  // Each group shown as text: its text, whose valid lines are already the
  // group's rules while text with errors waits, and the group's rules when
  // the text view opened, whose identities the lines keep while typing.
  const [ruleTexts, setRuleTexts] = useState<Record<string, { text: string; base: ChannelRule[] }>>({});
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
  const needsUpgrade = !config.policy;
  const settingsDirty = groupSettings !== null
    && JSON.stringify(groupSettings) !== JSON.stringify(policy.groups.find(value => value.id === groupSettings.id));
  /**
   * Parses a group's text against its rules when the text view opened, so
   * lines keep their identity and sort indexes however the text changed in
   * between; exits and states changed since come from the group's rules.
   */
  function parseRuleText(value: ChannelRuleGroup, text: string, base: ChannelRule[]): RuleTextResult {
    const existing = base.map((rule) => {
      const current = value.rules.find(item => item.id === rule.id);
      if (!current) return rule;
      const changed = current.enabled !== rule.enabled
        || current.exit.kind !== rule.exit.kind || current.exit.id !== rule.exit.id;
      return changed ? { ...rule, enabled: current.enabled, exit: current.exit } : rule;
    });
    const groups = policy.groups.map(item => item.id === value.id ? { ...item, rules: base } : item);
    return parseRules(text, {
      existing, format, nextIndex: nextRuleIndex({ ...policy, groups }),
      otherRemoteNames: groups.filter(item => item.id !== value.id)
        .flatMap(item => item.rules.flatMap(rule => rule.remote ? [rule.remote.name] : [])),
    });
  }
  const ruleTextResults = new Map(policy.groups.flatMap((value) => {
    const draft = ruleTexts[value.id];
    if (!draft) return [];
    return [[value.id, { text: draft.text, result: parseRuleText(value, draft.text, draft.base) }] as const];
  }));
  const invalidTextGroup = policy.groups.find(value => ruleTextResults.get(value.id)?.result.errors.length);
  const confirmNavigation = useUnsavedChanges(dirty || settingsDirty || Boolean(invalidTextGroup), () => {
    const restoredPolicy = initialChannelPolicy(channel, nodes);
    setPolicy(restoredPolicy);
    setConfig(channel.config);
    setFormat(channel.format);
    setName(channel.name);
    setSavedSignature(signature(restoredPolicy, channel.format, channel.config, channel.name));
    setGroupSettings(null);
    setRuleTexts({});
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
    return { policy: p };
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
    await render(p);
    if (lifeRef.current?.signal.aborted) return;
    const result = await client.updateSubscriptionChannel(
      channel.id,
      {
        name: name.trim(),
        enabled: channel.enabled,
        format,
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
    if (busy || invalidTextGroup) return;
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
    setPreviewError(null);
    try {
      const value = await render();
      if (!lifeRef.current?.signal.aborted) setPreview(value);
    } catch (reason) {
      if (!lifeRef.current?.signal.aborted) {
        setPreview(null);
        setPreviewError(reason);
      }
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
      groups: (value.enabled ? current.groups : removeGroupReferences(current.groups, value.id))
        .map((item) => item.id === value.id ? value : item),
    }));
  }
  /** Shows the group's rules as text, or as entries again. */
  function setRuleView(value: ChannelRuleGroup, view: 'entries' | 'text') {
    setRuleTexts((current) => {
      const next = { ...current };
      if (view === 'entries') delete next[value.id];
      else next[value.id] ??= { text: serializeRules(value.rules), base: value.rules };
      return next;
    });
  }
  /** Keeps the group's text; valid text becomes its rules at once. */
  function changeRuleText(value: ChannelRuleGroup, text: string) {
    const base = ruleTexts[value.id]?.base ?? value.rules;
    setRuleTexts(current => ({ ...current, [value.id]: { text, base } }));
    const parsed = parseRuleText(value, text, base);
    const changed = parsed.rules.length !== value.rules.length
      || parsed.rules.some((rule, index) => rule !== value.rules[index]);
    if (!parsed.errors.length && changed) updateGroup({ ...value, rules: parsed.rules });
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
    identity: { name: string; format: SubscriptionFormat },
  ) {
    setName(identity.name);
    setFormat(identity.format);
    setPolicy((current) => ({
      ...current,
      template: identity.format !== format && current.template
        ? { format: identity.format, content: channelTemplateDefaults[identity.format] }
        : current.template,
      selection: {
        ...current.selection,
        new_node_policy: p.selection.new_node_policy,
      },
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
    previewError,
    setPreviewError,
    setPreview,
    dirty,
    needsUpgrade,
    conflict,
    ruleTextResults,
    setRuleView,
    changeRuleText,
    invalidTextGroup,
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

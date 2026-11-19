import { useTranslation } from 'react-i18next';
import { useEffect, useRef, useState } from 'react';
import { toast } from '@shared/components/ui/toast-manager';

import type { ChannelRule, ChannelRuleGroup, SubscriptionChannel, SubscriptionChannelConfig, SubscriptionFormat, SubscriptionPreview } from '@/api/api-client';

import { useApiClient } from '@/api/api-client-context';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';
import { channelTemplateDefaults } from '@/constants/channel-templates';

import type { RuleTextResult } from './channel-rule-text';

import { parseRules, serializeRules } from './channel-rule-text';
import { incompatibleConfig, newRuleGroup, nextRuleIndex, removeGroupReferences } from './channel-policy';

export function useChannelDraft(channel: SubscriptionChannel, onSaved: (channel: SubscriptionChannel) => void) {
  const { t } = useTranslation();
  const client = useApiClient();
  const [config, setConfig] = useState(() => structuredClone(channel.config));
  const [format, setFormat] = useState<SubscriptionFormat>(channel.format);
  const [name, setName] = useState(channel.name);
  const [busy, setBusy] = useState(false);
  const [optionsOpen, setOptionsOpen] = useState(false);
  const [templateOpen, setTemplateOpen] = useState(false);
  const [groupID, setGroupID] = useState<string | null>(channel.config.groups[0]?.id ?? null);
  const [removeGroup, setRemoveGroup] = useState<string | null>(null);
  const [groupSettings, setGroupSettings] = useState<ChannelRuleGroup | null>(null);
  const [previewError, setPreviewError] = useState<unknown>(null);
  const [preview, setPreview] = useState<SubscriptionPreview | null>(null);
  // Each group shown as text: its text, whose valid lines are already the
  // group's rules while text with errors waits, and the group's rules when
  // the text view opened, whose identities the lines keep while typing.
  const [ruleTexts, setRuleTexts] = useState<Record<string, { text: string; base: ChannelRule[] }>>({});
  const lifeRef = useRef<AbortController | null>(null);
  const signature = (c: SubscriptionChannelConfig, f: SubscriptionFormat, n: string) => JSON.stringify({ c, f, n });
  const [savedSignature, setSavedSignature] = useState(() => signature(config, format, name));
  const dirty = signature(config, format, name) !== savedSignature;
  const settingsDirty = groupSettings !== null
    && JSON.stringify(groupSettings) !== JSON.stringify(config.groups.find(value => value.id === groupSettings.id));
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
    const groups = config.groups.map(item => item.id === value.id ? { ...item, rules: base } : item);
    return parseRules(text, {
      existing, format, nextIndex: nextRuleIndex({ ...config, groups }),
      otherRemoteNames: groups.filter(item => item.id !== value.id)
        .flatMap(item => item.rules.flatMap(rule => rule.remote ? [rule.remote.name] : [])),
    });
  }
  const ruleTextResults = new Map(config.groups.flatMap((value) => {
    const draft = ruleTexts[value.id];
    if (!draft) return [];
    return [[value.id, { text: draft.text, result: parseRuleText(value, draft.text, draft.base) }] as const];
  }));
  const invalidTextGroup = config.groups.find(value => ruleTextResults.get(value.id)?.result.errors.length);
  const confirmNavigation = useUnsavedChanges(dirty || settingsDirty || Boolean(invalidTextGroup), () => {
    const restored = structuredClone(channel.config);
    setConfig(restored);
    setFormat(channel.format);
    setName(channel.name);
    setSavedSignature(signature(restored, channel.format, channel.name));
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
  const group = config.groups.find((value) => value.id === groupID) ?? config.groups[0];
  const groupIndex = group ? config.groups.indexOf(group) : -1;
  const finalGroup = config.default_exit.kind === 'group' && config.default_exit.id === group?.id;
  const conflict = incompatibleConfig(config, format);
  async function render(c: SubscriptionChannelConfig = config, signal = lifeRef.current?.signal) {
    return client.previewSubscriptionChannel(channel.id, '', signal, { format, config: c });
  }
  async function persist(c: SubscriptionChannelConfig = config) {
    if (!name.trim() || c.groups.some((value) => !value.name.trim())) throw new Error(t('channels.nameRequired'));
    c = { ...c, groups: c.groups.map((value) => ({ ...value, name: value.name.trim() })) };
    await render(c);
    if (lifeRef.current?.signal.aborted) return;
    const result = await client.updateSubscriptionChannel(
      channel.id,
      { name: name.trim(), enabled: channel.enabled, format, config: c },
      channel.updated_at,
      lifeRef.current?.signal,
    );
    if (lifeRef.current?.signal.aborted) return;
    setConfig(c);
    onSaved(result);
    setSavedSignature(signature(c, format, name.trim()));
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
    const groups = [...config.groups];
    [groups[index], groups[index + delta]] = [groups[index + delta], groups[index]];
    setConfig({ ...config, groups });
  }
  function updateGroup(value: ChannelRuleGroup) {
    if (!value.enabled && config.default_exit.kind === 'group' && config.default_exit.id === value.id) {
      toast.add({ title: t('channels.groupExitInUse'), type: 'error' });
      return;
    }
    setConfig((current) => ({
      ...current,
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
    let number = config.groups.length + 1;
    let groupName = t('channels.newGroupName', { number });
    while (config.groups.some((value) => value.name === groupName)) groupName = t('channels.newGroupName', { number: ++number });
    const value = { ...newRuleGroup(), name: groupName };
    setConfig((current) => ({ ...current, groups: [...current.groups, value] }));
    setGroupID(value.id);
  }
  /** Applies the channel settings: its name and client, which brings that client's default template. */
  function applyOptions(identity: { name: string; format: SubscriptionFormat }) {
    setName(identity.name);
    setFormat(identity.format);
    setConfig((current) => ({
      ...current,
      template: identity.format !== format && current.template
        ? { format: identity.format, content: channelTemplateDefaults[identity.format] }
        : current.template,
    }));
  }
  return {
    config,
    setConfig,
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

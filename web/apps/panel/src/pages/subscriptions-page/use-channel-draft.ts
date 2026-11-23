import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import { toast } from '@shared/components/ui/toast-manager';

import type { ChannelRoute, ChannelRouteExit, ChannelRuleGroup, SubscriptionChannel, SubscriptionChannelConfig, SubscriptionFormat, SubscriptionPreview } from '@/api/api-client';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { describeRequestError } from '@/components/error-notice';
import { channelTemplateDefaults } from '@/constants/channel-templates';

import { groupInUse, incompatibleConfig, newRuleGroup, releaseFinalExit, remapRuleSets, removeGroupReferences } from './channel-policy';

export function useChannelDraft(channel: SubscriptionChannel, onSaved: (channel: SubscriptionChannel) => void) {
  const { t } = useTranslation();
  const client = useApiClient();
  const ruleSets = useQuery(queries.ruleSets(client)).data;
  const [config, setConfig] = useState(() => structuredClone(channel.config));
  const [format, setFormat] = useState<SubscriptionFormat>(channel.format);
  const [name, setName] = useState(channel.name);
  const [busy, setBusy] = useState(false);
  const [optionsOpen, setOptionsOpen] = useState(false);
  const [templateOpen, setTemplateOpen] = useState(false);
  /** The group shown, or null for the channel's exit rules. */
  const [groupID, setGroupID] = useState<string | null>(null);
  const [removeGroup, setRemoveGroup] = useState<string | null>(null);
  const [groupSettings, setGroupSettings] = useState<ChannelRuleGroup | null>(null);
  const [previewError, setPreviewError] = useState<unknown>(null);
  const [preview, setPreview] = useState<SubscriptionPreview | null>(null);
  const lifeRef = useRef<AbortController | null>(null);
  const signature = (c: SubscriptionChannelConfig, f: SubscriptionFormat, n: string) => JSON.stringify({ c, f, n });
  const [savedSignature, setSavedSignature] = useState(() => signature(config, format, name));
  const dirty = signature(config, format, name) !== savedSignature;
  const settingsDirty = groupSettings !== null
    && JSON.stringify(groupSettings) !== JSON.stringify(config.groups.find(value => value.id === groupSettings.id));
  const confirmNavigation = useUnsavedChanges(dirty || settingsDirty, () => {
    const restored = structuredClone(channel.config);
    setConfig(restored);
    setFormat(channel.format);
    setName(channel.name);
    setSavedSignature(signature(restored, channel.format, channel.name));
    setGroupSettings(null);
    setOptionsOpen(false);
    setTemplateOpen(false);
  }, busy);
  useEffect(() => {
    const controller = new AbortController();
    lifeRef.current = controller;
    return () => controller.abort();
  }, []);
  const group = groupID === null ? undefined : config.groups.find(value => value.id === groupID);
  const groupIndex = group ? config.groups.indexOf(group) : -1;
  const conflict = incompatibleConfig(config, format, ruleSets);
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
  /** Says why a group an exit rule uses can be neither deleted nor disabled; true when it is in use. */
  function refuseInUse(id: string): boolean {
    if (!groupInUse(config, id)) return false;
    toast.add({ title: t('channels.groupExitInUse'), type: 'error' });
    return true;
  }
  function updateGroup(value: ChannelRuleGroup) {
    if (!value.enabled && refuseInUse(value.id)) return;
    setConfig((current) => ({
      ...(value.enabled ? current : releaseFinalExit(current, value.id)),
      groups: (value.enabled ? current.groups : removeGroupReferences(current.groups, value.id))
        .map((item) => item.id === value.id ? value : item),
    }));
  }
  /** Asks to delete a group, unless an exit rule uses it. */
  function requestRemoveGroup(id: string) {
    if (!refuseInUse(id)) setRemoveGroup(id);
  }
  function confirmRemoveGroup() {
    if (removeGroup === null) return;
    setConfig((current) => ({
      ...releaseFinalExit(current, removeGroup),
      groups: removeGroupReferences(current.groups.filter((value) => value.id !== removeGroup), removeGroup),
    }));
    if (groupID === removeGroup) setGroupID(null);
    setRemoveGroup(null);
  }
  function setRoutes(routes: ChannelRoute[]) {
    setConfig((current) => ({ ...current, routes }));
  }
  function setDefaultExit(exit: ChannelRouteExit) {
    setConfig((current) => ({ ...current, default_exit: exit }));
  }
  function addGroup() {
    let number = config.groups.length + 1;
    let groupName = t('channels.newGroupName', { number });
    while (config.groups.some((value) => value.name === groupName)) groupName = t('channels.newGroupName', { number: ++number });
    const value = { ...newRuleGroup(), name: groupName };
    setConfig((current) => ({ ...current, groups: [...current.groups, value] }));
    setGroupID(value.id);
  }
  /**
   * Applies the channel settings: its name and client. A new client brings
   * its default template in place of a template of the channel's own, and
   * each exit rule takes the new client's rule set of the same name.
   */
  function applyOptions(identity: { name: string; format: SubscriptionFormat }) {
    const switched = identity.format !== format;
    setName(identity.name);
    setFormat(identity.format);
    setConfig((current) => ({
      ...current,
      routes: switched && ruleSets ? remapRuleSets(current.routes, ruleSets, identity.format) : current.routes,
      template: switched && current.template
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
    ruleSets,
    optionsOpen,
    setOptionsOpen,
    templateOpen,
    setTemplateOpen,
    group,
    groupIndex,
    groupSettings,
    setGroupSettings,
    setGroupID,
    removeGroup,
    setRemoveGroup,
    requestRemoveGroup,
    confirmRemoveGroup,
    preview,
    previewError,
    setPreviewError,
    setPreview,
    dirty,
    conflict,
    confirmNavigation,
    render,
    save,
    showPreview,
    reorder,
    updateGroup,
    setRoutes,
    setDefaultExit,
    addGroup,
    applyOptions,
  };
}

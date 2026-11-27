import { arrayMove } from '@dnd-kit/sortable';
import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import { toast } from '@shared/components/ui/toast-manager';

import type { ChannelRoute, ChannelRouteExit, ChannelRuleGroup, SubscriptionChannel, SubscriptionChannelConfig, SubscriptionFormat, SubscriptionPreview } from '@/api/api-client';

import { queries } from '@/api/queries';
import { useApiClient } from '@/api/api-client-context';
import { toastFailure } from '@/components/error-notice';
import { useUnsavedChanges } from '@/hooks/use-unsaved-changes';
import { channelTemplateDefaults } from '@/constants/channel-templates';

import { groupInUse, incompatibleConfig, newRuleGroup, releaseFinalExit, remapRuleSets, removeGroupReferences } from './channel-policy';

/** A channel's unsaved draft as it waits out a reload, with the version it was made on. */
interface StoredDraft {
  base: string;
  name: string;
  format: SubscriptionFormat;
  config: SubscriptionChannelConfig;
}

/** Where a channel's draft waits out a reload: this tab's session storage, which a closed tab clears. */
function draftKey(channelID: string) {
  return `sing-box-panel.channel-draft.${channelID}`;
}

/**
 * The draft a reload left for the channel: restored when it was made on the
 * version at hand, outdated when the channel changed since. Reading it
 * changes nothing, as a component may be rendered twice.
 */
function readStoredDraft(channel: SubscriptionChannel): { draft?: StoredDraft; outdated: boolean } {
  try {
    const raw = window.sessionStorage.getItem(draftKey(channel.id));
    if (raw === null) return { outdated: false };
    const draft = JSON.parse(raw) as Partial<StoredDraft> | null;
    if (typeof draft?.base !== 'string' || typeof draft.name !== 'string' || typeof draft.format !== 'string' || !Array.isArray(draft.config?.groups)) {
      return { outdated: false };
    }
    return draft.base === channel.updated_at ? { draft: draft as StoredDraft, outdated: false } : { outdated: true };
  } catch {
    return { outdated: false };
  }
}

function storeDraft(channelID: string, draft: StoredDraft | null) {
  try {
    if (draft) window.sessionStorage.setItem(draftKey(channelID), JSON.stringify(draft));
    else window.sessionStorage.removeItem(draftKey(channelID));
  } catch {
    // Without storage a reload loses the draft, as it would anyway.
  }
}

export function useChannelDraft(channel: SubscriptionChannel, onSaved: (channel: SubscriptionChannel) => void) {
  const { t } = useTranslation();
  const client = useApiClient();
  const ruleSets = useQuery(queries.ruleSets(client)).data;
  // A browser may reload without asking, as an embedded one does, so an unsaved draft outlives the reload.
  const [stored] = useState(() => readStoredDraft(channel));
  const [config, setConfig] = useState(() => structuredClone(stored.draft?.config ?? channel.config));
  const [format, setFormat] = useState<SubscriptionFormat>(stored.draft?.format ?? channel.format);
  const [name, setName] = useState(stored.draft?.name ?? channel.name);
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
  const [savedSignature, setSavedSignature] = useState(() => signature(channel.config, channel.format, channel.name));
  const dirty = signature(config, format, name) !== savedSignature;
  const settingsDirty = groupSettings !== null
    && JSON.stringify(groupSettings) !== JSON.stringify(config.groups.find(value => value.id === groupSettings.id));
  function discard() {
    const restored = structuredClone(channel.config);
    setConfig(restored);
    setFormat(channel.format);
    setName(channel.name);
    setSavedSignature(signature(restored, channel.format, channel.name));
    setGroupSettings(null);
    setOptionsOpen(false);
    setTemplateOpen(false);
  }
  const discardRef = useRef(discard);
  useEffect(() => {
    discardRef.current = discard;
  });
  useUnsavedChanges(dirty || settingsDirty, discard, busy);
  // The draft is kept while unsaved and dropped once saved or discarded;
  // leaving the channel drops it, a reload does not.
  useEffect(() => {
    storeDraft(channel.id, dirty ? { base: channel.updated_at, name, format, config } : null);
  }, [channel.id, channel.updated_at, config, dirty, format, name]);
  useEffect(() => () => storeDraft(channel.id, null), [channel.id]);
  // Says once that a reload's draft came back, with the way to drop it, or that it could not.
  const noticedRef = useRef(false);
  useEffect(() => {
    if (noticedRef.current) return;
    noticedRef.current = true;
    if (stored.draft && dirty) {
      toast.add({
        title: t('channels.draftRestored'),
        type: 'info',
        actionProps: { children: t('common.unsaved.discard'), onClick: () => discardRef.current() },
      });
    } else if (stored.outdated) {
      toast.add({ title: t('channels.draftOutdated'), type: 'warning' });
    }
  }, [dirty, stored, t]);
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
      if (!lifeRef.current?.signal.aborted) toastFailure(t('channels.saveFailed'), reason);
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
  /** Moves a group to the place of another, as a drag drops it. */
  function moveGroup(active: string, over: string) {
    const ids = config.groups.map(group => group.id);
    const from = ids.indexOf(active);
    const to = ids.indexOf(over);
    if (from < 0 || to < 0 || from === to) return;
    setConfig({ ...config, groups: arrayMove(config.groups, from, to) });
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
  /** Asks to delete a group, which an exit rule that uses it refuses; true when it asks. */
  function requestRemoveGroup(id: string): boolean {
    if (refuseInUse(id)) return false;
    setRemoveGroup(id);
    return true;
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
    render,
    save,
    showPreview,
    moveGroup,
    updateGroup,
    setRoutes,
    setDefaultExit,
    addGroup,
    applyOptions,
  };
}

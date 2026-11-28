import type { PropsWithChildren } from 'react';

import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { toast } from '@shared/components/ui/toast-manager';
import { act, renderHook, waitFor } from '@testing-library/react';

import type { ApiClient, SubscriptionChannel } from '@/api/api-client';

import '@/i18n';
import { deferred } from '@/tests/deferred';
import { TestRouter } from '@/tests/test-router';
import { ApiClientProvider } from '@/api/api-client-context';
import { channelTemplateDefaults } from '@/constants/channel-templates';
import { useChannelDraft } from '@/pages/subscriptions-page/use-channel-draft';
import { createMockApiClient, testSubscriptionChannels } from '@/tests/api/mock-api-client';
import { emptyChannelConfig, newRuleGroup } from '@/pages/subscriptions-page/channel-policy';

const channel: SubscriptionChannel = {
  ...testSubscriptionChannels[0], format: 'sing-box',
  config: emptyChannelConfig(),
};

function mount(value = channel, overrides: Partial<ApiClient> = {}) {
  const client = createMockApiClient({
    updateSubscriptionChannel: vi.fn(async (_id, input) => ({ ...value, ...input, updated_at: 'updated' })),
    ...overrides,
  });
  const onSaved = vi.fn();
  const hook = renderHook(() => {
    const [saved, setSaved] = useState(value);
    return useChannelDraft(saved, result => {
      onSaved(result);
      setSaved(result);
    });
  }, {
    wrapper: ({ children }: PropsWithChildren) => (
      <TestRouter><ApiClientProvider client={client}>{children}</ApiClientProvider></TestRouter>
    ),
  });
  return { ...hook, client, onSaved };
}

describe('channel draft', () => {
  it('retains edits across group selection and uses the accepted revision for the next save', async () => {
    const first = { ...newRuleGroup([{ kind: 'node', id: 'first' }]), name: 'First' };
    const second = { ...newRuleGroup([{ kind: 'node', id: 'second' }]), name: 'Second' };
    const value = { ...channel, config: { ...channel.config, groups: [first, second] } };
    const { result, client } = mount(value);
    const edited = { ...first, name: 'Renamed', members: [{ kind: 'node' as const, id: 'third' }] };
    act(() => result.current.updateGroup(edited));
    act(() => result.current.setGroupID(second.id));
    expect(result.current.group).toEqual(second);
    act(() => result.current.setGroupID(first.id));
    expect(result.current.group).toEqual(edited);
    await act(() => result.current.save());
    expect(client.updateSubscriptionChannel.mock.calls[0][1].config.groups).toEqual([edited, second]);
    act(() => result.current.updateGroup({ ...edited, name: 'Another edit' }));
    await act(() => result.current.save());
    expect(client.updateSubscriptionChannel.mock.calls[1][2]).toBe('updated');
  });
  it('saves in one CAS request, which the server renders before it stores', async () => {
    const { result, client, onSaved } = mount(channel);
    act(() => result.current.addGroup());
    await act(() => result.current.save());
    expect(client.previewSubscriptionChannel).not.toHaveBeenCalled();
    expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(channel.id, expect.objectContaining({
      enabled: channel.enabled,
      config: result.current.config,
    }), channel.updated_at, expect.any(AbortSignal));
    expect(onSaved).toHaveBeenCalledOnce();
    expect(result.current.dirty).toBe(false);
  });
  it('keeps configuration changes local and retains them after a rejected save', async () => {
    const { result, client, onSaved } = mount(channel, { updateSubscriptionChannel: vi.fn().mockRejectedValue(new Error('conflict')) });
    act(() => result.current.addGroup());
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
    await act(() => result.current.save());
    expect(result.current.config.groups).toHaveLength(1);
    expect(result.current.dirty).toBe(true);
    expect(result.current.busy).toBe(false);
    expect(onSaved).not.toHaveBeenCalled();
  });
  it('aborts a pending save on unmount and ignores its late success', async () => {
    const pending = deferred<SubscriptionChannel>();
    const { result, client, unmount, onSaved } = mount(channel, {
      updateSubscriptionChannel: vi.fn().mockReturnValue(pending.promise),
    });
    let saving!: Promise<void>;
    act(() => {
      saving = result.current.save();
    });
    await waitFor(() => expect(client.updateSubscriptionChannel).toHaveBeenCalledOnce());
    unmount();
    expect(client.updateSubscriptionChannel.mock.calls[0][3]?.aborted).toBe(true);
    await act(async () => {
      pending.resolve({ ...channel, updated_at: 'updated' });
      await saving;
    });
    expect(onSaved).not.toHaveBeenCalled();
  });
  it('previews unsaved templates without persisting them', async () => {
    const { result, client } = mount();
    act(() => result.current.setConfig(c => ({ ...c, template: { format: 'sing-box', content: '{}' } })));
    await act(() => result.current.showPreview());
    expect(client.previewSubscriptionChannel).toHaveBeenCalledWith(channel.id, '', expect.any(AbortSignal), expect.objectContaining({
      config: expect.objectContaining({ template: { format: 'sing-box', content: '{}' } }),
    }));
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });
  it('changes formats atomically, replaces a format-specific template and preserves group edits', () => {
    const { result } = mount();
    act(() => {
      result.current.addGroup();
    });
    act(() => result.current.setConfig(c => ({ ...c, template: { format: 'sing-box', content: '{}' } })));
    const group = result.current.group;
    act(() => result.current.applyOptions({ name: 'Loon', format: 'loon' }));
    expect(result.current.config.groups).toEqual([group]);
    expect(result.current.config.template).toEqual({ format: 'loon', content: channelTemplateDefaults.loon });
  });
  it('protects groups an exit rule uses, frees the final exit of a disabled group and reorders whole group records', () => {
    const first = { ...newRuleGroup([]), name: 'First' };
    const second = { ...newRuleGroup([]), name: 'Second' };
    const config = {
      ...channel.config, groups: [first, second], default_exit: { kind: 'group' as const, id: first.id },
      routes: [{ id: 'route', enabled: true, rule: { kind: 'domain' as const, value: 'example.com' }, exit: { kind: 'group' as const, id: second.id } }],
    };
    const { result } = mount({ ...channel, config });
    act(() => result.current.updateGroup({ ...first, enabled: false }));
    expect(result.current.config.default_exit).toEqual({ kind: 'direct' });
    act(() => result.current.requestRemoveGroup(second.id));
    expect(result.current.removeGroup).toBeNull();
    act(() => result.current.moveGroup(first.id, second.id));
    expect(result.current.config.groups).toEqual([second, { ...first, enabled: false }]);
  });
  it('points exit rules at the new client’s rule set of the same name when the client changes', async () => {
    const sets = [
      { id: 'sing-box-streaming', client: 'sing-box' as const, name: 'Streaming', rules: [], channels: [], created_at: 'created', updated_at: 'updated' },
      { id: 'mihomo-streaming', client: 'mihomo' as const, name: 'streaming', rules: [], channels: [], created_at: 'created', updated_at: 'updated' },
      { id: 'sing-box-ads', client: 'sing-box' as const, name: 'Ads', rules: [], channels: [], created_at: 'created', updated_at: 'updated' },
    ];
    const routes = sets.filter(set => set.client === 'sing-box').map(set => ({ id: set.id, enabled: true, rule_set_id: set.id, exit: { kind: 'direct' as const } }));
    const { result } = mount({ ...channel, config: { ...channel.config, routes } }, {
      listSubscriptionRuleSets: vi.fn().mockResolvedValue({ items: sets }),
    });
    await waitFor(() => expect(result.current.ruleSets).toBeDefined());
    expect(result.current.conflict).toBe(false);
    act(() => result.current.applyOptions({ name: channel.name, format: 'mihomo' }));
    expect(result.current.config.routes.map(route => route.rule_set_id)).toEqual(['mihomo-streaming', 'sing-box-ads']);
    // The set mihomo lacks stays and blocks saving.
    expect(result.current.conflict).toBe(true);
  });
});

it('removes references in the same draft update when disabling a group', () => {
  const child = { ...newRuleGroup([]), id: 'child', name: 'Child' };
  const parent = { ...newRuleGroup([{ kind: 'group', id: 'child' }]), id: 'parent', name: 'Parent' };
  const { result } = mount({ ...channel, config: { ...channel.config, groups: [parent, child], default_exit: { kind: 'group', id: 'parent' } } });
  act(() => result.current.updateGroup({ ...child, enabled: false }));
  expect(result.current.config.groups[1].enabled).toBe(false);
  expect(result.current.config.groups[0].members).toEqual([]);
  expect(result.current.config.default_exit).toEqual({ kind: 'group', id: 'parent' });
});

it('keeps an unsaved draft through a reload of the same version, and drops one the channel outdated', async () => {
  const key = `sing-box-panel.channel-draft.${channel.id}`;
  const feedback = vi.spyOn(toast, 'add');
  const first = mount();
  act(() => first.result.current.addGroup());
  const left = window.sessionStorage.getItem(key);
  expect(JSON.parse(left!)).toMatchObject({ base: channel.updated_at, config: { groups: [expect.any(Object)] } });
  // A reload runs no cleanup, so the draft it left is still there.
  first.unmount();
  window.sessionStorage.setItem(key, left!);

  const reloaded = mount();
  expect(reloaded.result.current.dirty).toBe(true);
  expect(reloaded.result.current.config.groups).toHaveLength(1);
  expect(feedback).toHaveBeenLastCalledWith(expect.objectContaining({ title: 'Restored the changes left unsaved before the reload', type: 'info' }));
  await act(() => reloaded.result.current.save());
  expect(window.sessionStorage.getItem(key)).toBeNull();
  reloaded.unmount();

  window.sessionStorage.setItem(key, JSON.stringify({ ...JSON.parse(left!), base: 'an earlier version' }));
  const outdated = mount();
  expect(outdated.result.current.dirty).toBe(false);
  expect(feedback).toHaveBeenLastCalledWith(expect.objectContaining({ type: 'warning' }));
  expect(window.sessionStorage.getItem(key)).toBeNull();
  feedback.mockRestore();
});

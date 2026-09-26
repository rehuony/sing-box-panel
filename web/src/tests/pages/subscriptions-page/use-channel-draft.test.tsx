import type { PropsWithChildren } from 'react';

import { useState } from 'react';
import { describe, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';

import type { ApiClient, SubscriptionChannel, SubscriptionPreview } from '@/api/api-client';

import '@/i18n';
import { deferred } from '@/tests/deferred';
import { TestRouter } from '@/tests/test-router';
import { ApiClientProvider } from '@/api/api-client-context';
import { channelTemplateDefaults } from '@/constants/channel-templates';
import { useChannelDraft } from '@/pages/subscriptions-page/use-channel-draft';
import { createMockApiClient, testSubscriptionChannels } from '@/tests/api/mock-api-client';
import { initialChannelPolicy, newRuleGroup } from '@/pages/subscriptions-page/channel-policy';

const channel: SubscriptionChannel = {
  ...testSubscriptionChannels[0], format: 'sing-box',
  config: { policy: initialChannelPolicy({ config: {} }, []) },
};

function mount(value = channel, overrides: Partial<ApiClient> = {}) {
  const client = createMockApiClient({
    updateSubscriptionChannel: vi.fn(async (_id, input) => ({ ...value, ...input, updated_at: 'updated' })),
    ...overrides,
  });
  const onSaved = vi.fn();
  const hook = renderHook(() => {
    const [saved, setSaved] = useState(value);
    return useChannelDraft(saved, [], result => {
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
    const first = { ...newRuleGroup(['first']), name: 'First' };
    const second = { ...newRuleGroup(['second']), name: 'Second' };
    const value = { ...channel, config: { policy: { ...channel.config.policy!, groups: [first, second] } } };
    const { result, client } = mount(value);
    const edited = { ...first, name: 'Renamed', rules: [
      { id: 'rule', enabled: true, kind: 'domain' as const, value: 'example.com', exit: { kind: 'group-default' as const } },
    ] };
    act(() => result.current.updateGroup(edited));
    act(() => result.current.setGroupID(second.id));
    expect(result.current.group).toEqual(second);
    act(() => result.current.setGroupID(first.id));
    expect(result.current.group).toEqual(edited);
    await act(() => result.current.save());
    expect(client.updateSubscriptionChannel.mock.calls[0][1].config.policy?.groups).toEqual([edited, second]);
    act(() => result.current.updateGroup({ ...edited, name: 'Another edit' }));
    await act(() => result.current.save());
    expect(client.updateSubscriptionChannel.mock.calls[1][2]).toBe('updated');
  });
  it.each([false, true])('validates only native clients before a CAS save (native: %s)', async native => {
    const { result, client, onSaved } = mount(channel, { supportsNativeChannelValidation: native });
    act(() => result.current.addGroup());
    await act(() => result.current.save());
    expect(client.previewSubscriptionChannel).toHaveBeenCalledTimes(native ? 1 : 0);
    expect(client.updateSubscriptionChannel).toHaveBeenCalledWith(channel.id, expect.objectContaining({
      enabled: channel.enabled, public_host: channel.public_host,
      config: expect.objectContaining({ policy: result.current.policy }),
    }), channel.updated_at, expect.any(AbortSignal));
    expect(onSaved).toHaveBeenCalledOnce();
    expect(result.current.dirty).toBe(false);
  });
  it('saves the same complete policy used by preview when editing a legacy channel', async () => {
    const legacy = { ...channel, format: 'loon' as const, enabled: false, config: {} };
    const { result, client } = mount(legacy);
    act(() => result.current.applyOptions(result.current.policy, { name: '  Renamed  ', format: 'loon' }));
    await act(() => result.current.save());
    expect(client.updateSubscriptionChannel.mock.calls[0][1]).toMatchObject({ name: 'Renamed', enabled: false, config: { policy: result.current.policy } });
    act(() => result.current.addGroup());
    await act(() => result.current.save());
    expect(client.updateSubscriptionChannel.mock.calls[1][1].config.policy?.groups).toHaveLength(1);
  });
  it('keeps policy changes local and retains them after a rejected save', async () => {
    const { result, client, onSaved } = mount(channel, { updateSubscriptionChannel: vi.fn().mockRejectedValue(new Error('conflict')) });
    act(() => result.current.addGroup());
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
    await act(() => result.current.save());
    expect(result.current.policy.groups).toHaveLength(1);
    expect(result.current.dirty).toBe(true);
    expect(result.current.busy).toBe(false);
    expect(onSaved).not.toHaveBeenCalled();
  });
  it('does not persist or discard edits when native validation rejects the draft', async () => {
    const { result, client } = mount(channel, { supportsNativeChannelValidation: true, previewSubscriptionChannel: vi.fn().mockRejectedValue(new Error('invalid')) });
    act(() => result.current.addGroup());
    await act(() => result.current.save());
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
    expect(result.current.policy.groups).toHaveLength(1);
    expect(result.current.dirty).toBe(true);
  });
  it('aborts pending validation on unmount and ignores its late success', async () => {
    const pending = deferred<SubscriptionPreview>();
    const { result, client, unmount, onSaved } = mount(channel, {
      supportsNativeChannelValidation: true,
      previewSubscriptionChannel: vi.fn().mockReturnValue(pending.promise),
    });
    let saving!: Promise<void>;
    act(() => {
      saving = result.current.save();
    });
    unmount();
    expect(client.previewSubscriptionChannel.mock.calls[0][2]?.aborted).toBe(true);
    await act(async () => {
      pending.resolve({} as SubscriptionPreview);
      await saving;
    });
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
  });
  it('previews unsaved templates without persisting them', async () => {
    const { result, client } = mount();
    act(() => result.current.setPolicy(p => ({ ...p, template: { format: 'sing-box', content: '{}' } })));
    await act(() => result.current.showPreview());
    expect(client.previewSubscriptionChannel).toHaveBeenCalledWith(channel.id, '', expect.any(AbortSignal), expect.objectContaining({
      config: expect.objectContaining({ policy: expect.objectContaining({ template: { format: 'sing-box', content: '{}' } }) }),
    }));
    expect(client.updateSubscriptionChannel).not.toHaveBeenCalled();
  });
  it('changes formats atomically, replaces a format-specific template and preserves group edits', () => {
    const { result } = mount();
    act(() => {
      result.current.addGroup();
    });
    act(() => result.current.setPolicy(p => ({ ...p, template: { format: 'sing-box', content: '{}' } })));
    const group = result.current.group;
    act(() => result.current.applyOptions(result.current.policy, { name: 'Loon', format: 'loon' }));
    expect(result.current.policy.groups).toEqual([group]);
    expect(result.current.policy.template).toEqual({ format: 'loon', content: channelTemplateDefaults.loon });
  });
  it('merges group members into selection without losing unrelated exclusions or changing the fallback', () => {
    const group = { ...newRuleGroup([]), name: 'Group' };
    const policy = { ...channel.config.policy!, groups: [group], default_exit: { kind: 'reject' as const }, selection: { ids: ['existing'], excluded_ids: ['new', 'hidden'], new_node_policy: 'exclude' as const } };
    const { result } = mount({ ...channel, config: { policy } });
    act(() => result.current.updateGroup({ ...group, node_ids: ['new'] }));
    expect(result.current.policy.selection).toEqual({ ids: ['existing', 'new'], excluded_ids: ['hidden'], new_node_policy: 'exclude' });
    act(() => result.current.addGroup());
    expect(result.current.policy.default_exit).toEqual({ kind: 'reject' });
  });
  it('protects an enabled final group and reorders whole group records', () => {
    const first = { ...newRuleGroup([]), name: 'First' };
    const second = { ...newRuleGroup([]), name: 'Second' };
    const policy = { ...channel.config.policy!, groups: [first, second], default_exit: { kind: 'group' as const, id: first.id } };
    const { result } = mount({ ...channel, config: { policy } });
    act(() => result.current.updateGroup({ ...first, enabled: false }));
    expect(result.current.policy.groups[0]).toEqual(first);
    act(() => result.current.reorder(0, 1));
    expect(result.current.policy.groups).toEqual([second, first]);
  });
});

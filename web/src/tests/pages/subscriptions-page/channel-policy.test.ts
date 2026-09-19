import { describe, expect, it } from 'vitest';

import type { ChannelPolicy } from '@/api/api-client';

import { canAccelerateRuleURL, directRuleURL, effectiveRuleURL, incompatiblePolicy, newRuleGroup, toggleChannelNode } from '@/pages/subscriptions-page/channel-policy';

const policy: ChannelPolicy = { selection: { ids: ['one'], excluded_ids: [], new_node_policy: 'include' }, organizer: { prefix: '', exclude_names: [], deduplicate: false, sort: 'none', incompatible: 'error' }, groups: [], default_exit: { kind: 'direct' } };
describe('channel editing policy', () => {
  it('normalizes known wrappers and preserves escaped URLs when toggling acceleration', () => {
    const origin = 'https://raw.githubusercontent.com/a/r/main/a%20b.json?x=%2F';
    const wrapped = `https://ghproxy.net/https://gh-proxy.com/${origin}`;
    expect(directRuleURL(wrapped)).toBe(origin);
    expect(effectiveRuleURL(wrapped, true)).toBe(`https://gh-proxy.com/${origin}`);
    expect(effectiveRuleURL(wrapped, false)).toBe(origin);
    for (const invalid of ['', 'https://github.com.evil.example/a', 'https://user:password@github.com/a', 'https://other.example/a']) expect(canAccelerateRuleURL(invalid)).toBe(false);
  });
  it('keeps explicit deselection when newly discovered nodes are included', () => {
    const removed = toggleChannelNode(policy, 'one', false);
    expect(removed.selection).toEqual({ ids: [], excluded_ids: ['one'], new_node_policy: 'include' });
    expect(toggleChannelNode(removed, 'one', true).selection).toEqual(policy.selection);
    expect(policy.selection.ids).toEqual(['one']);
  });
  it('takes a fixed candidate snapshot for each new group', () => {
    const ids = ['one', 'two'];
    const group = newRuleGroup(ids);
    ids.push('later');
    expect(group.node_ids).toEqual(['one', 'two']);
    expect(group.default_exit).toEqual({ kind: 'node', id: 'one' });
  });
  it('flags incompatible metadata without converting or discarding its URL', () => {
    const group = newRuleGroup(['one']);
    group.rules = [{ id: 'rule', enabled: true, kind: 'remote', exit: { kind: 'group-default' }, remote: { name: 'Rules', url: 'https://example.com/native', format: 'mrs', behavior: 'domain', accelerated: false, update_interval: 3600 } }];
    const p = { ...policy, groups: [group] };
    expect(incompatiblePolicy(p, 'sing-box')).toBe(true);
    expect(incompatiblePolicy(p, 'mihomo')).toBe(false);
    expect(group.rules[0].remote?.url).toBe('https://example.com/native');
  });
});

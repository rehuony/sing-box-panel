import { describe, expect, it } from 'vitest';

import type { ChannelPolicy } from '@/api/api-client';

import { canAccelerateRuleURL, compareChannelRules, directRuleURL, effectiveRuleURL, incompatiblePolicy, initialChannelPolicy, newRuleGroup, nextRuleIndex, updateGroupCandidates } from '@/pages/subscriptions-page/channel-policy';

const policy: ChannelPolicy = { selection: { ids: ['one'], excluded_ids: [], new_node_policy: 'include' }, groups: [], default_exit: { kind: 'direct' } };
describe('channel editing policy', () => {
  it('repairs exits after removing members and preserves remaining candidate order', () => {
    const group = {
      ...newRuleGroup(['one', 'two']),
      candidate_order: ['node:two', 'node:one'],
      rules: [{ id: 'rule', kind: 'domain_suffix' as const, value: 'example.com', enabled: true, exit: { kind: 'node' as const, id: 'one' } }],
    };
    const next = updateGroupCandidates(group, ['two']);
    expect(next.node_ids).toEqual(['two']);
    expect(next.candidate_order).toEqual(['node:two']);
    expect(next).not.toHaveProperty('default_exit');
    expect(next.rules[0].exit).toEqual({ kind: 'group-default' });
    expect(group.node_ids).toEqual(['one', 'two']);
    expect(updateGroupCandidates(next, []).candidate_order).toEqual([]);
  });
  it('preserves built-in candidate order without an independent default', () => {
    const group = { ...newRuleGroup(['one']), builtin_nodes: ['direct' as const] };
    expect(updateGroupCandidates(group, ['one', 'two']).candidate_order).toEqual(['node:one', 'builtin:direct', 'node:two']);
    const next = updateGroupCandidates(group, [], ['reject'], ['node:one', 'builtin:reject']);
    expect(next.node_ids).toEqual([]);
    expect(next.candidate_order).toEqual(['builtin:reject']);
  });
  it('assigns legacy priorities without mutating saved data and allows stable name ties', () => {
    const rule = { id: 'old', kind: 'domain' as const, enabled: true, value: 'z.example', exit: { kind: 'group-default' as const } };
    const group = { ...newRuleGroup([]), rules: [rule, { ...rule, id: 'early', value: 'a.example', sort_index: 10 }] };
    const initial = initialChannelPolicy({ config: { policy: { ...policy, groups: [group] } } }, []);
    expect(initial.groups[0].rules[0].sort_index).toBe(10);
    expect(rule).not.toHaveProperty('sort_index');
    expect(nextRuleIndex(initial)).toBe(20);
    const duplicate = { ...initial.groups[0].rules[1], id: 'duplicate' };
    expect([...initial.groups[0].rules, duplicate].sort(compareChannelRules).map(value => value.id)).toEqual(['early', 'duplicate', 'old']);
  });
  it('normalizes known wrappers and preserves escaped URLs when toggling acceleration', () => {
    const origin = 'https://raw.githubusercontent.com/a/r/main/a%20b.json?x=%2F';
    const wrapped = `https://ghproxy.net/https://gh-proxy.com/${origin}`;
    expect(directRuleURL(wrapped)).toBe(origin);
    expect(effectiveRuleURL(wrapped, true)).toBe(`https://gh-proxy.com/${origin}`);
    expect(effectiveRuleURL(wrapped, false)).toBe(origin);
    for (const invalid of ['', 'https://github.com.evil.example/a', 'https://user:password@github.com/a', 'https://other.example/a']) expect(canAccelerateRuleURL(invalid)).toBe(false);
  });
  it('takes a fixed candidate snapshot for each new group', () => {
    const ids = ['one', 'two'];
    const group = newRuleGroup(ids);
    ids.push('later');
    expect(group.type).toBe('select');
    expect(group.builtin_nodes).toEqual([]);
    expect(group.node_ids).toEqual(['one', 'two']);
    expect(group.candidate_order).toEqual(['node:one', 'node:two']);
    expect(newRuleGroup([]).candidate_order).toEqual([]);
  });
  it('flags incompatible metadata without converting or discarding its URL', () => {
    const group = newRuleGroup(['one']);
    group.rules = [{ id: 'rule', enabled: true, kind: 'remote', exit: { kind: 'group-default' }, remote: { name: 'Rules', url: 'https://example.com/native', format: 'mrs', behavior: 'domain', accelerated: false, update_interval: 3600 } }];
    const p = { ...policy, groups: [group] };
    expect(incompatiblePolicy(p, 'sing-box')).toBe(true);
    expect(incompatiblePolicy(p, 'mihomo')).toBe(false);
    expect(incompatiblePolicy(p, 'loon')).toBe(true);
    group.rules[0].remote!.format = 'text';
    expect(incompatiblePolicy(p, 'loon')).toBe(true);
    expect(group.rules[0].remote?.url).toBe('https://example.com/native');
  });
  it('flags unsupported strategies and built-in nodes when changing clients', () => {
    const group = newRuleGroup(['one']);
    const p = { ...policy, groups: [group] };
    group.type = 'fallback';
    expect(incompatiblePolicy(p, 'sing-box')).toBe(true);
    expect(incompatiblePolicy(p, 'mihomo')).toBe(false);
    expect(incompatiblePolicy(p, 'loon')).toBe(false);
    group.type = 'url-test';
    expect(incompatiblePolicy(p, 'sing-box')).toBe(false);
    group.builtin_nodes = ['direct', 'reject'];
    expect(incompatiblePolicy(p, 'sing-box')).toBe(true);
    expect(incompatiblePolicy(p, 'mihomo')).toBe(false);
    expect(incompatiblePolicy(p, 'loon')).toBe(true);
    group.type = 'select';
    expect(incompatiblePolicy(p, 'loon')).toBe(false);
  });
});

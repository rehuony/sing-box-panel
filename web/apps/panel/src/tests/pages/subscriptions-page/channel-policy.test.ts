import { describe, expect, it } from 'vitest';

import type { ChannelGroupMember, SubscriptionChannelConfig } from '@/api/api-client';

import { canAccelerateRuleURL, compareChannelRules, createsGroupCycle, detectRuleFormat, directRuleURL, effectiveRuleURL, emptyChannelConfig, incompatibleConfig, newRuleGroup, nextRuleIndex, removeGroupReferences, updateGroupMembers } from '@/pages/subscriptions-page/channel-policy';

const node = (id: string): ChannelGroupMember => ({ kind: 'node', id });
const group = (id: string): ChannelGroupMember => ({ kind: 'group', id });
describe('channel editing policy', () => {
  it('repairs exits after removing members and keeps the remaining members in order', () => {
    const value = {
      ...newRuleGroup([node('two'), node('one')]),
      rules: [{ id: 'rule', kind: 'domain_suffix' as const, value: 'example.com', enabled: true, exit: { kind: 'node' as const, id: 'one' } }],
    };
    const next = updateGroupMembers(value, [node('two')]);
    expect(next.members).toEqual([node('two')]);
    expect(next.rules[0].exit).toEqual({ kind: 'group-default' });
    expect(value.members).toEqual([node('two'), node('one')]);
  });
  it('sorts signed indices with zero as the default and preserves name ties', () => {
    const rule = { id: 'zero', kind: 'domain' as const, enabled: true, value: 'z.example', exit: { kind: 'group-default' as const } };
    const rules = [rule, { ...rule, id: 'early', value: 'a.example', sort_index: -10 }, { ...rule, id: 'tie', value: 'a.example', sort_index: -10 }];
    const initial: SubscriptionChannelConfig = { ...emptyChannelConfig(), groups: [{ ...newRuleGroup(), rules }] };
    expect([...rules].sort(compareChannelRules).map(value => value.id)).toEqual(['early', 'tie', 'zero']);
    expect(nextRuleIndex(initial)).toBe(10);
    initial.groups[0].rules = [{ ...rule, sort_index: -20 }];
    expect(nextRuleIndex(initial)).toBe(-10);
    initial.groups[0].rules[0].sort_index = Number.MAX_SAFE_INTEGER;
    expect(nextRuleIndex(initial)).toBe(Number.MAX_SAFE_INTEGER);
  });
  it.each([
    ['mihomo', 'rules.YAML?download=1', 'yaml'], ['mihomo', 'rules.yml', 'yaml'],
    ['mihomo', 'rules.MRS#section', 'mrs'], ['mihomo', 'rules.txt', 'text'], ['mihomo', 'rules.list', 'text'],
    ['sing-box', 'rules.json', 'source'], ['sing-box', 'rules.srs', 'binary'],
    ['loon', 'rules.list', 'loon'], ['loon', 'rules.txt', 'loon'], ['loon', 'rules.loon', 'loon'],
    ['mihomo', 'rules.srs', ''], ['sing-box', 'rules.mrs', ''], ['loon', 'rules.yaml', ''],
    ['mihomo', 'yaml', ''], ['sing-box', 'download?file=rules.srs', ''],
    ['mihomo', 'rules.constructor', ''], ['sing-box', 'rules.__proto__', ''],
  ] as const)('infers %s from URL path %s', (client, path, expected) => {
    expect(detectRuleFormat(`https://gh-proxy.com/https://raw.githubusercontent.com/a/r/main/${path}`, client)).toBe(expected);
  });
  it('detects indirect cycles and removes references without disturbing other members', () => {
    const leaf = { ...newRuleGroup([node('one')]), id: 'leaf' };
    const middle = { ...newRuleGroup([group('leaf')]), id: 'middle' };
    const parent = { ...newRuleGroup([group('middle'), node('two'), group('leaf')]), id: 'parent' };
    const groups = [parent, middle, leaf];
    expect(createsGroupCycle(groups, 'leaf', 'parent')).toBe(true);
    expect(createsGroupCycle(groups, 'parent', 'leaf')).toBe(false);
    const cleaned = removeGroupReferences(groups, 'leaf');
    expect(cleaned[0].members).toEqual([group('middle'), node('two')]);
    expect(cleaned[1].members).toEqual([]);
    expect(cleaned[2]).toBe(leaf);
    expect(middle.members).toEqual([group('leaf')]);
  });
  it('normalizes known wrappers and preserves escaped URLs when toggling acceleration', () => {
    const origin = 'https://raw.githubusercontent.com/a/r/main/a%20b.json?x=%2F';
    const wrapped = `https://ghproxy.net/https://gh-proxy.com/${origin}`;
    expect(directRuleURL(wrapped)).toBe(origin);
    expect(effectiveRuleURL(wrapped, true)).toBe(`https://gh-proxy.com/${origin}`);
    expect(effectiveRuleURL(wrapped, false)).toBe(origin);
    for (const invalid of ['', 'https://github.com.evil.example/a', 'https://user:password@github.com/a', 'https://other.example/a']) expect(canAccelerateRuleURL(invalid)).toBe(false);
  });
  it('flags incompatible metadata without converting or discarding its URL', () => {
    const value = newRuleGroup([node('one')]);
    value.rules = [{ id: 'rule', enabled: true, kind: 'remote', exit: { kind: 'group-default' }, remote: { name: 'Rules', url: 'https://example.com/native', format: 'mrs', behavior: 'domain', accelerated: false } }];
    const config = { ...emptyChannelConfig(), groups: [value] };
    expect(incompatibleConfig(config, 'sing-box')).toBe(true);
    expect(incompatibleConfig(config, 'mihomo')).toBe(false);
    expect(incompatibleConfig(config, 'loon')).toBe(true);
    value.rules[0].remote!.format = 'text';
    expect(incompatibleConfig(config, 'loon')).toBe(true);
    expect(value.rules[0].remote?.url).toBe('https://example.com/native');
  });
  it('flags unsupported strategies and built-in nodes when changing clients', () => {
    const value = newRuleGroup([node('one')]);
    const config = { ...emptyChannelConfig(), groups: [value] };
    value.type = 'fallback';
    expect(incompatibleConfig(config, 'sing-box')).toBe(true);
    expect(incompatibleConfig(config, 'mihomo')).toBe(false);
    expect(incompatibleConfig(config, 'loon')).toBe(false);
    value.type = 'url-test';
    expect(incompatibleConfig(config, 'sing-box')).toBe(false);
    value.members = [node('one'), { kind: 'builtin', id: 'direct' }, { kind: 'builtin', id: 'reject' }];
    expect(incompatibleConfig(config, 'sing-box')).toBe(true);
    expect(incompatibleConfig(config, 'mihomo')).toBe(false);
    expect(incompatibleConfig(config, 'loon')).toBe(true);
    value.type = 'select';
    expect(incompatibleConfig(config, 'loon')).toBe(false);
  });
});

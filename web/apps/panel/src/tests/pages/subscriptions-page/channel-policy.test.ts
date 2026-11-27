import { describe, expect, it } from 'vitest';

import type { ChannelGroupMember, ChannelRoute, ChannelRuleGroup, SubscriptionNode, SubscriptionRuleSet } from '@/api/api-client';

import { canAccelerateRuleURL, createsGroupCycle, detectRuleFormat, directRuleURL, effectiveRuleURL, emptyChannelConfig, groupTypeName, incompatibleConfig, matchedNodes, newRuleGroup, removeGroupReferences } from '@/pages/subscriptions-page/channel-policy';

const node = (id: string): ChannelGroupMember => ({ kind: 'node', id });
const group = (id: string): ChannelGroupMember => ({ kind: 'group', id });
function ruleSet(id: string, client: SubscriptionRuleSet['client'], name: string): SubscriptionRuleSet {
  return { id, client, name, icon: 'layers', rules: [], channels: [], created_at: '2026-11-20T00:00:00Z', updated_at: '2026-11-20T00:00:00Z' };
}
function poolNode(id: string, protocol: string, origin: SubscriptionNode['origin']): SubscriptionNode {
  return { id, display_name: id, origin_name: id, protocol, origin, updated_at: '2026-11-20T00:00:00Z' };
}
describe('channel editing policy', () => {
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
  it('flags exit rules the client cannot use without converting or discarding them', () => {
    const sets = [ruleSet('mihomo-set', 'mihomo', 'Streaming'), ruleSet('sing-box-set', 'sing-box', 'Streaming')];
    const remote: ChannelRoute = { id: 'remote', enabled: true, exit: { kind: 'direct' }, rule: { kind: 'remote', remote: { name: 'Rules', url: 'https://example.com/native', format: 'mrs', behavior: 'domain', accelerated: false } } };
    const config = { ...emptyChannelConfig(), routes: [remote] };
    expect(incompatibleConfig(config, 'sing-box', sets)).toBe(true);
    expect(incompatibleConfig(config, 'mihomo', sets)).toBe(false);
    remote.rule!.remote!.behavior = 'classical';
    expect(incompatibleConfig(config, 'mihomo', sets)).toBe(true);
    expect(remote.rule?.remote?.url).toBe('https://example.com/native');

    config.routes = [{ id: 'set', enabled: true, exit: { kind: 'direct' }, rule_set_id: 'mihomo-set' }];
    expect(incompatibleConfig(config, 'mihomo', sets)).toBe(false);
    expect(incompatibleConfig(config, 'sing-box', sets)).toBe(true);
    expect(incompatibleConfig(config, 'mihomo', [])).toBe(true);
    // Rule sets not yet read flag nothing.
    expect(incompatibleConfig(config, 'sing-box', undefined)).toBe(false);
  });
  it('flags unsupported strategies and built-in nodes when changing clients', () => {
    const value = newRuleGroup([node('one')]);
    const config = { ...emptyChannelConfig(), groups: [value] };
    value.type = 'fallback';
    expect(incompatibleConfig(config, 'sing-box', [])).toBe(true);
    expect(incompatibleConfig(config, 'mihomo', [])).toBe(false);
    expect(incompatibleConfig(config, 'loon', [])).toBe(false);
    value.type = 'url-test';
    expect(incompatibleConfig(config, 'sing-box', [])).toBe(false);
    value.members = [node('one'), { kind: 'builtin', id: 'direct' }, { kind: 'builtin', id: 'reject' }];
    expect(incompatibleConfig(config, 'sing-box', [])).toBe(true);
    expect(incompatibleConfig(config, 'mihomo', [])).toBe(false);
    expect(incompatibleConfig(config, 'loon', [])).toBe(true);
    value.type = 'select';
    expect(incompatibleConfig(config, 'loon', [])).toBe(false);
  });

  it('names group types as each client\'s configuration writes them', () => {
    expect(['select', 'url-test', 'fallback'].map(type => groupTypeName(type as ChannelRuleGroup['type'], 'sing-box'))).toEqual(['selector', 'urltest', 'fallback']);
    expect(['select', 'url-test', 'fallback'].map(type => groupTypeName(type as ChannelRuleGroup['type'], 'loon'))).toEqual(['select', 'url-test', 'fallback']);
  });
  it('matches pool nodes by origin, protocol and name rules after the members', () => {
    const fleet = { kind: 'inbound' as const, node_id: 'edge', node_name: 'edge', inbound_tag: 'in' };
    const source = { kind: 'source' as const, source_id: 'one', source_name: 'One' };
    const nodes = [poolNode('HK vless', 'vless', fleet), poolNode('hk trojan', 'trojan', source), poolNode('JP vless', 'vless', source), poolNode('hk member', 'vless', source)];
    const value: ChannelRuleGroup = { ...newRuleGroup([node('hk member')]), match: { include: ['hk'], exclude: ['/trojan$/'], origins: [], protocols: [] } };
    expect(matchedNodes(value, nodes).map(item => item.id)).toEqual(['HK vless']);
    value.match = { include: ['/[/', 'vless'], exclude: [], origins: ['source:one'], protocols: ['vless'] };
    expect(matchedNodes(value, nodes).map(item => item.id)).toEqual(['JP vless']);
    value.match = { include: [], exclude: [], origins: [], protocols: [] };
    expect(matchedNodes(value, nodes)).toEqual([]);
  });
});

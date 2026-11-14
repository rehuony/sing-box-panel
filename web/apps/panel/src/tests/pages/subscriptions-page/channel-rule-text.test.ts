import { describe, expect, it } from 'vitest';

import type { ChannelRule } from '@/api/api-client';

import { parseRules, serializeRules } from '@/pages/subscriptions-page/channel-rule-text';

const existing: ChannelRule[] = [
  { id: 'r-suffix', enabled: true, sort_index: 30, kind: 'domain_suffix', value: 'example.com', exit: { kind: 'node', id: 'node-a' } },
  { id: 'r-ip6', enabled: false, sort_index: 10, kind: 'ip_cidr', value: '2001:db8::/32', exit: { kind: 'direct' } },
  {
    id: 'r-remote', enabled: true, sort_index: 20, kind: 'remote', exit: { kind: 'group-default' },
    remote: { name: 'ads', url: 'https://raw.githubusercontent.com/o/r/main/ads.yaml', format: 'yaml', behavior: 'domain', accelerated: true },
  },
];
const options = { existing, format: 'mihomo' as const, nextIndex: 100, otherRemoteNames: ['streaming'] };

describe('channel rule text', () => {
  it('returns the same rules after a round trip through text', () => {
    const text = serializeRules(existing);
    expect(text).toBe([
      'IP-CIDR6,2001:db8::/32',
      'RULE-SET,ads,https://gh-proxy.com/https://raw.githubusercontent.com/o/r/main/ads.yaml,yaml,domain',
      'DOMAIN-SUFFIX,example.com',
    ].join('\n'));
    const result = parseRules(text, options);
    expect(result.errors).toEqual([]);
    expect(result.rules).toEqual(existing);
    expect(result.rules[0]).toBe(existing[0]);
  });

  it('reads back rule sets whose links hold commas', () => {
    const remote = (id: string, name: string, url: string, format: 'yaml' | 'source', behavior?: 'classical', accelerated = false): ChannelRule => ({
      id, enabled: true, sort_index: 10, kind: 'remote', exit: { kind: 'group-default' },
      remote: { name, url, format, ...(behavior ? { behavior } : {}), accelerated },
    });
    const mihomo = [
      remote('r-path', 'path', 'https://example.com/a,b/rules.yaml?x=1,2', 'yaml', 'classical'),
      remote('r-github', 'github', 'https://raw.githubusercontent.com/o/r/main/a,b.yaml', 'yaml', 'classical', true),
    ];
    const singBox = [remote('r-query', 'query', 'https://example.com/rules.json?tags=ads,trackers', 'source')];
    for (const [rules, format] of [[mihomo, 'mihomo'], [singBox, 'sing-box']] as const) {
      const text = serializeRules(rules);
      const result = parseRules(text, { existing: rules, format, nextIndex: 100, otherRemoteNames: [] });
      expect(result.errors).toEqual([]);
      expect(result.rules).toEqual(rules);
    }
  });

  it('reads a pasted Clash list and keeps what it can of each rule', () => {
    const valid = parseRules([
      '# exits come from the group',
      '  - DOMAIN-SUFFIX,example.com,Proxy',
      '  - "IP-CIDR,10.0.0.0/8,DIRECT,no-resolve"',
      '// another comment',
      '',
      'domain-keyword,tracker',
    ].join('\n'), { ...options, existing: [existing[0]!] });
    expect(valid.errors).toEqual([]);
    expect(valid.notes).toEqual([
      { line: 2, code: 'policy_dropped', detail: 'Proxy' },
      { line: 3, code: 'policy_dropped', detail: 'DIRECT' },
      { line: 3, code: 'no_resolve_dropped' },
    ]);
    // The matching rule keeps its identity and exit; new lines follow the
    // group's indexes from nextIndex.
    expect(valid.rules.map(rule => [rule.kind, rule.value, rule.sort_index, rule.exit.kind])).toEqual([
      ['domain_suffix', 'example.com', 30, 'node'],
      ['ip_cidr', '10.0.0.0/8', 100, 'group-default'],
      ['domain_keyword', 'tracker', 110, 'group-default'],
    ]);
    expect(valid.rules[0]!.id).toBe('r-suffix');
  });

  it('reassigns the group sort indexes in line order', () => {
    const text = ['DOMAIN-SUFFIX,example.com', 'IP-CIDR6,2001:db8::/32'].join('\n');
    const result = parseRules(text, options);
    expect(result.errors).toEqual([]);
    expect(result.rules.map(rule => [rule.id, rule.sort_index, rule.enabled])).toEqual([
      ['r-suffix', 10, true],
      ['r-ip6', 20, false],
    ]);
  });

  it('reports each line that cannot become a rule', () => {
    const result = parseRules([
      'rules:',
      'GEOIP,CN,DIRECT',
      'PROCESS-NAME,curl,DIRECT',
      'DOMAIN,',
      'IP-CIDR,300.1.1.1/8',
      'DOMAIN,https://example.com',
      'RULE-SET,streaming,https://example.com/streaming.yaml',
      'RULE-SET,lists,https://example.com/lists',
      'RULE-SET,binary,https://example.com/x.srs,binary',
      'RULE-SET,fast,https://example.com/fast.mrs,mrs,classical',
      'RULE-SET,local,file:///rules.yaml',
      'RULE-SET,proxy,https://example.com/proxy.yaml,PROXY',
    ].join('\n'), options);
    expect(result.rules).toEqual([]);
    expect(result.errors).toEqual([
      { line: 1, code: 'unsupported_type', detail: 'rules:' },
      { line: 2, code: 'unsupported_type', detail: 'GEOIP' },
      { line: 3, code: 'unsupported_type', detail: 'PROCESS-NAME' },
      { line: 4, code: 'missing_value' },
      { line: 5, code: 'invalid_ip', detail: '300.1.1.1/8' },
      { line: 6, code: 'invalid_domain', detail: 'https://example.com' },
      { line: 7, code: 'duplicate_name', detail: 'streaming' },
      { line: 8, code: 'format_required' },
      { line: 9, code: 'format_unsupported', detail: 'binary' },
      { line: 10, code: 'mrs_classical' },
      { line: 11, code: 'invalid_url', detail: 'file:///rules.yaml' },
      // A policy after a rule set's link reads as part of the link.
      { line: 12, code: 'format_required' },
    ]);
  });
});

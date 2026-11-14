import type { ChannelRemoteRuleSet, ChannelRule, SubscriptionFormat } from '@/api/api-client';

import { compareChannelRules, detectRuleFormat, directRuleURL, effectiveRuleURL, ruleFormats } from './channel-policy';

/** Why a line of rule text cannot become a rule. */
export type RuleTextErrorCode
  = | 'unsupported_type'
    | 'missing_value'
    | 'invalid_domain'
    | 'invalid_ip'
    | 'missing_name'
    | 'duplicate_name'
    | 'invalid_url'
    | 'format_required'
    | 'format_unsupported'
    | 'behavior_unsupported'
    | 'mrs_classical';

/** What parsing left out of a line that still became a rule. */
export type RuleTextNoteCode = 'policy_dropped' | 'no_resolve_dropped';

export interface RuleTextIssue<Code extends string> {
  code: Code;
  /** The line's number, from 1. */
  line: number;
  /** The value the message names, such as the rule type or policy. */
  detail?: string;
}

export interface RuleTextOptions {
  /** The first sort index for lines beyond the group's existing rules. */
  nextIndex: number;
  /** The group's rules, whose identity, exit and state matching lines keep. */
  existing: ChannelRule[];
  format: SubscriptionFormat;
  /** Rule-set names other groups of the channel use. */
  otherRemoteNames: string[];
}

export interface RuleTextResult {
  rules: ChannelRule[];
  notes: RuleTextIssue<RuleTextNoteCode>[];
  errors: RuleTextIssue<RuleTextErrorCode>[];
}

const typeKinds: Record<string, Exclude<ChannelRule['kind'], 'remote'>> = {
  'DOMAIN': 'domain',
  'DOMAIN-SUFFIX': 'domain_suffix',
  'DOMAIN-KEYWORD': 'domain_keyword',
  'IP-CIDR': 'ip_cidr',
  'IP-CIDR6': 'ip_cidr',
};
const kindTypes: Record<Exclude<ChannelRule['kind'], 'remote' | 'ip_cidr'>, string> = {
  domain: 'DOMAIN',
  domain_suffix: 'DOMAIN-SUFFIX',
  domain_keyword: 'DOMAIN-KEYWORD',
};
const allFormats: readonly ChannelRemoteRuleSet['format'][] = ['source', 'binary', 'yaml', 'text', 'mrs', 'loon'];
const behaviors: readonly NonNullable<ChannelRemoteRuleSet['behavior']>[] = ['domain', 'ipcidr', 'classical'];

/**
 * Writes rules as Clash classic lines, one per rule in priority order. A
 * rule set is `RULE-SET,name,link,format[,behavior]`, its link accelerated
 * as the rule says. Exits and disabled states are not written; parsing the
 * text with the same rules keeps them.
 */
export function serializeRules(rules: readonly ChannelRule[]): string {
  return [...rules].sort(compareChannelRules).map((rule) => {
    if (rule.kind === 'remote' && rule.remote) {
      const { accelerated, behavior, format, name, url } = rule.remote;
      return ['RULE-SET', name, effectiveRuleURL(url, accelerated), format, ...(behavior ? [behavior] : [])].join(',');
    }
    const value = rule.value ?? '';
    if (rule.kind === 'ip_cidr') return `${value.includes(':') ? 'IP-CIDR6' : 'IP-CIDR'},${value}`;
    return `${kindTypes[rule.kind as keyof typeof kindTypes]},${value}`;
  }).join('\n');
}

/**
 * Reads Clash classic lines into rules. Blank lines and lines starting with
 * `#` or `//` are skipped; a YAML list marker and quotes around a line are
 * removed. A trailing `no-resolve`, and a policy name after a match value,
 * are dropped with a note, since the group decides the exit; a rule set's
 * link may hold commas. A line that matches an existing rule by
 * type and value, or a rule set by name, keeps that rule's ID, exit and
 * enabled state. Lines take the group's existing sort indexes in order;
 * further lines continue from `nextIndex`. Rules keep the order of the
 * existing ones first, so unchanged text returns the existing rules.
 */
export function parseRules(text: string, options: RuleTextOptions): RuleTextResult {
  const result: RuleTextResult = { rules: [], errors: [], notes: [] };
  const unmatched = new Map<string, ChannelRule[]>();
  for (const rule of options.existing) {
    const key = ruleKey(rule);
    unmatched.set(key, [...(unmatched.get(key) ?? []), rule]);
  }
  const indexes = options.existing.map(rule => rule.sort_index ?? 0).sort((left, right) => left - right);
  const names = new Set(options.otherRemoteNames);
  const parsed: { rule: ChannelRule; original?: ChannelRule }[] = [];
  text.split(/\r?\n/).forEach((raw, position) => {
    const line = position + 1;
    const content = unwrapLine(raw);
    if (!content || content.startsWith('#') || content.startsWith('//')) return;
    const fields = content.split(',').map(field => field.trim());
    const type = fields[0]!.toUpperCase();
    const error = (code: RuleTextErrorCode, detail?: string) =>
      result.errors.push({ line, code, ...(detail ? { detail } : {}) });
    let rule: ChannelRule;
    let trailing: string[];
    if (type === 'RULE-SET') {
      const remote = parseRemote(content.split(','), options.format, error);
      if (!remote) return;
      if (names.has(remote.value.name)) {
        error('duplicate_name', remote.value.name);
        return;
      }
      names.add(remote.value.name);
      rule = { id: '', enabled: true, kind: 'remote', remote: remote.value, exit: { kind: 'group-default' } };
      trailing = remote.noResolve ? ['no-resolve'] : [];
    } else if (Object.hasOwn(typeKinds, type)) {
      const kind = typeKinds[type]!;
      const value = fields[1] ?? '';
      if (!value) {
        error('missing_value');
        return;
      }
      if (kind === 'ip_cidr' ? !validAddressOrPrefix(value) : /[\s/:@]/.test(value)) {
        error(kind === 'ip_cidr' ? 'invalid_ip' : 'invalid_domain', value);
        return;
      }
      rule = { id: '', enabled: true, kind, value, exit: { kind: 'group-default' } };
      trailing = fields.slice(2);
    } else {
      error('unsupported_type', fields[0]);
      return;
    }
    for (const field of trailing.filter(Boolean)) {
      result.notes.push(field.toLowerCase() === 'no-resolve'
        ? { line, code: 'no_resolve_dropped' }
        : { line, code: 'policy_dropped', detail: field });
    }
    const original = unmatched.get(ruleKey(rule))?.shift();
    parsed.push({ rule, original });
  });
  if (result.errors.length) return result;
  const positions = new Map(options.existing.map((rule, index) => [rule.id, index]));
  result.rules = parsed
    .map(({ original, rule }, index) => {
      const sortIndex = index < indexes.length ? indexes[index]! : options.nextIndex + (index - indexes.length) * 10;
      const value: ChannelRule = original
        ? { ...rule, id: original.id, enabled: original.enabled, exit: original.exit, sort_index: sortIndex }
        : { ...rule, id: crypto.randomUUID(), sort_index: sortIndex };
      return original && sameRule(value, original) ? original : value;
    })
    .map((rule, index) => ({ rule, position: positions.get(rule.id) ?? options.existing.length + index }))
    .sort((left, right) => left.position - right.position)
    .map(({ rule }) => rule);
  return result;
}

/** Removes a YAML list marker and quotes around the line. */
function unwrapLine(raw: string): string {
  let content = raw.trim().replace(/^-\s+/, '');
  if (content.length > 1 && (content[0] === '"' || content[0] === '\'') && content.at(-1) === content[0]) {
    content = content.slice(1, -1).trim();
  }
  return content;
}

/**
 * Reads `RULE-SET,name,link[,format[,behavior]][,no-resolve]` from both ends:
 * names cannot hold commas, but links can, so the link is everything between
 * the name and the trailing fields, joined again with commas.
 */
function parseRemote(
  parts: string[],
  client: SubscriptionFormat,
  error: (code: RuleTextErrorCode, detail?: string) => void,
): { value: ChannelRemoteRuleSet; noResolve: boolean } | null {
  const name = parts[1]?.trim() ?? '';
  if (!name) {
    error('missing_name');
    return null;
  }
  let end = parts.length;
  // Each trailing field is taken only while a link field remains before it.
  const trailing = (accepts: (field: string) => boolean): string | undefined => {
    const field = end > 3 ? parts[end - 1]!.trim().toLowerCase() : '';
    if (!field || !accepts(field)) return undefined;
    end -= 1;
    return field;
  };
  const noResolve = trailing(field => field === 'no-resolve') !== undefined;
  let behavior = trailing(field => (behaviors as readonly string[]).includes(field)) as ChannelRemoteRuleSet['behavior'];
  const named = trailing(field => (allFormats as readonly string[]).includes(field)) as ChannelRemoteRuleSet['format'] | undefined;
  const raw = parts.slice(2, end).join(',').trim();
  const url = directRuleURL(raw);
  if (/\s/.test(raw) || !validRuleURL(url)) {
    error('invalid_url', raw || undefined);
    return null;
  }
  if (named && !ruleFormats(client).includes(named)) {
    error('format_unsupported', named);
    return null;
  }
  const format = named ?? detectRuleFormat(url, client);
  if (!format) {
    error('format_required');
    return null;
  }
  if (behavior && client !== 'mihomo') {
    error('behavior_unsupported', behavior);
    return null;
  }
  if (client === 'mihomo') behavior ??= 'domain';
  if (format === 'mrs' && behavior === 'classical') {
    error('mrs_classical');
    return null;
  }
  return {
    value: { name, url, format, ...(behavior ? { behavior } : {}), accelerated: url !== raw },
    noResolve,
  };
}

function validRuleURL(value: string): boolean {
  try {
    const url = new URL(value);
    return ['http:', 'https:'].includes(url.protocol) && !url.username && !url.password && !url.hash;
  } catch {
    return false;
  }
}

/** An IPv4 or IPv6 address, or a prefix of one. */
function validAddressOrPrefix(value: string): boolean {
  const [address, length, extra] = value.split('/');
  if (extra !== undefined || !address) return false;
  const ipv4 = /^(?:(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)\.){3}(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)$/.test(address);
  const ipv6 = !ipv4 && address.includes(':') && /^[0-9a-f:.]+$/i.test(address) && URL.canParse(`http://[${address}]/`);
  if (!ipv4 && !ipv6) return false;
  return length === undefined || (/^\d{1,3}$/.test(length) && Number(length) <= (ipv4 ? 32 : 128));
}

function ruleKey(rule: ChannelRule): string {
  return rule.kind === 'remote' ? `remote\0${rule.remote?.name ?? ''}` : `${rule.kind}\0${rule.value ?? ''}`;
}

function sameRule(left: ChannelRule, right: ChannelRule): boolean {
  const remote = (rule: ChannelRule) => rule.remote
    ? [rule.remote.name, rule.remote.url, rule.remote.format, rule.remote.behavior ?? '', rule.remote.accelerated]
    : null;
  return left.id === right.id && left.enabled === right.enabled && (left.sort_index ?? 0) === (right.sort_index ?? 0)
    && left.kind === right.kind && (left.value ?? '') === (right.value ?? '')
    && JSON.stringify(remote(left)) === JSON.stringify(remote(right))
    && left.exit.kind === right.exit.kind && left.exit.id === right.exit.id;
}

import type { ChannelRemoteRuleSet, SubscriptionFormat, SubscriptionRule } from '@/api/api-client';

import { detectRuleFormat, directRuleURL, effectiveRuleURL, ruleFormats } from './channel-policy';

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

export interface RuleTextResult {
  /** The line each rule came from, from 1. */
  lines: number[];
  /** The rules of the lines that could be read, in order. */
  rules: SubscriptionRule[];
  notes: RuleTextIssue<RuleTextNoteCode>[];
  errors: RuleTextIssue<RuleTextErrorCode>[];
}

const typeKinds: Record<string, Exclude<SubscriptionRule['kind'], 'remote'>> = {
  'DOMAIN': 'domain',
  'DOMAIN-SUFFIX': 'domain_suffix',
  'DOMAIN-KEYWORD': 'domain_keyword',
  'IP-CIDR': 'ip_cidr',
  'IP-CIDR6': 'ip_cidr',
};
const kindTypes: Record<SubscriptionRule['kind'], string> = {
  domain: 'DOMAIN',
  domain_suffix: 'DOMAIN-SUFFIX',
  domain_keyword: 'DOMAIN-KEYWORD',
  ip_cidr: 'IP-CIDR',
  remote: 'RULE-SET',
};
const allFormats: readonly ChannelRemoteRuleSet['format'][] = ['source', 'binary', 'yaml', 'text', 'mrs', 'loon'];
const behaviors: readonly NonNullable<ChannelRemoteRuleSet['behavior']>[] = ['domain', 'ipcidr', 'classical'];

/** A rule's kind as its classic rule type, the name the interface gives it untranslated. */
export function ruleType(kind: SubscriptionRule['kind']): string {
  return kindTypes[kind];
}

/** One rule as a classic rule line; a remote rule set is `RULE-SET,name,link,format[,behavior]`. */
export function ruleLine(rule: SubscriptionRule): string {
  if (rule.kind === 'remote' && rule.remote) {
    const { accelerated, behavior, format, name, url } = rule.remote;
    return ['RULE-SET', name, effectiveRuleURL(url, accelerated), format, ...(behavior ? [behavior] : [])].join(',');
  }
  const value = rule.value ?? '';
  if (rule.kind === 'ip_cidr') return `${value.includes(':') ? 'IP-CIDR6' : 'IP-CIDR'},${value}`;
  return `${kindTypes[rule.kind]},${value}`;
}

/** A rule as a message names it, without its type: its match value, or a remote rule set's name. */
export function ruleName(rule: SubscriptionRule): string {
  return rule.kind === 'remote' ? rule.remote?.name ?? '' : rule.value ?? '';
}

/** Writes rules as classic rule lines, one per rule in order. */
export function serializeRules(rules: readonly SubscriptionRule[]): string {
  return rules.map(ruleLine).join('\n');
}

/** A line as a classic rule line: one that does not start with a rule type takes the kind's. */
export function withRuleType(line: string, kind: Exclude<SubscriptionRule['kind'], 'remote'>): string {
  const type = unwrapLine(line).split(',')[0]!.trim().toUpperCase();
  return type === 'RULE-SET' || Object.hasOwn(typeKinds, type) ? line : `${kindTypes[kind]},${line.trim()}`;
}

/**
 * Reads classic rule lines into rules, in order. Blank lines and lines
 * starting with `#` or `//` are skipped; a YAML list marker and quotes
 * around a line are removed. A trailing `no-resolve`, and a policy name
 * after a match value, are dropped with a note, since the exit rule that
 * uses the rules decides the exit; a rule set's link may hold commas.
 * Remote rule sets need distinct names. Text with errors is not a rule
 * list yet; its readable lines still come back, for marking and pasting.
 */
export function parseRules(text: string, format: SubscriptionFormat): RuleTextResult {
  const result: RuleTextResult = { rules: [], lines: [], errors: [], notes: [] };
  const names = new Set<string>();
  text.split(/\r?\n/).forEach((raw, position) => {
    const line = position + 1;
    const content = unwrapLine(raw);
    if (!content || content.startsWith('#') || content.startsWith('//')) return;
    const fields = content.split(',').map(field => field.trim());
    const type = fields[0]!.toUpperCase();
    const error = (code: RuleTextErrorCode, detail?: string) =>
      result.errors.push({ line, code, ...(detail ? { detail } : {}) });
    let rule: SubscriptionRule;
    let trailing: string[];
    if (type === 'RULE-SET') {
      const remote = parseRemote(content.split(','), format, error);
      if (!remote) return;
      if (names.has(remote.value.name)) {
        error('duplicate_name', remote.value.name);
        return;
      }
      names.add(remote.value.name);
      rule = { kind: 'remote', remote: remote.value };
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
      rule = { kind, value };
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
    result.rules.push(rule);
    result.lines.push(line);
  });
  return result;
}

/**
 * What a part of a rule line is, which its colour shows: the rule type, a
 * match value or an address, a remote rule set's name, link and format or
 * behavior, punctuation and comments, a field the rule drops, or text that
 * is not a rule line.
 */
export type RuleTone = 'type' | 'value' | 'address' | 'name' | 'link' | 'option' | 'mark' | 'dropped' | 'plain';

export interface RuleToken {
  text: string;
  tone: RuleTone;
}

/**
 * A line of rule text in its parts, read as parseRules reads it; the parts'
 * texts join back into the line, so they can be drawn under it. A line whose
 * type is not a rule type is one plain part.
 */
export function ruleLineTokens(raw: string): RuleToken[] {
  const tokens: RuleToken[] = [];
  const push = (text: string, tone: RuleTone) => {
    if (text) tokens.push({ text, tone });
  };
  let start = raw.length - raw.trimStart().length;
  let end = raw.trimEnd().length;
  push(raw.slice(0, start), 'plain');
  const marker = /^-\s+/.exec(raw.slice(start, end));
  if (marker) {
    push(marker[0], 'mark');
    start += marker[0].length;
  }
  const quote = raw[start];
  const quoted = (quote === '"' || quote === '\'') && end - start > 1 && raw[end - 1] === quote;
  if (quoted) {
    push(quote, 'mark');
    start += 1;
    end -= 1;
  }
  const body = raw.slice(start, end);
  const content = body.trim();
  if (content.startsWith('#') || content.startsWith('//')) {
    push(body, 'mark');
  } else {
    const parts = body.split(',');
    const type = parts[0]!.trim().toUpperCase();
    let groups: { parts: string[]; tone: RuleTone }[] = [];
    if (type === 'RULE-SET') {
      // The link is everything between the name and the trailing fields, as parseRemote reads it.
      let last = parts.length;
      const trailing = (accepts: (field: string) => boolean) => {
        if (last > 3 && accepts(parts[last - 1]!.trim().toLowerCase())) last -= 1;
      };
      trailing(field => field === 'no-resolve');
      trailing(field => (behaviors as readonly string[]).includes(field));
      trailing(field => (allFormats as readonly string[]).includes(field));
      groups = [
        { parts: parts.slice(0, 1), tone: 'type' },
        { parts: parts.slice(1, 2), tone: 'name' },
        { parts: parts.slice(2, last), tone: 'link' },
        ...parts.slice(last).map(part => ({ parts: [part], tone: part.trim().toLowerCase() === 'no-resolve' ? 'dropped' as const : 'option' as const })),
      ];
    } else if (Object.hasOwn(typeKinds, type)) {
      groups = parts.map((part, index) => {
        let tone: RuleTone = 'dropped';
        if (index === 0) tone = 'type';
        else if (index === 1) tone = typeKinds[type] === 'ip_cidr' ? 'address' : 'value';
        return { parts: [part], tone };
      });
    } else {
      groups = [{ parts: [body], tone: 'plain' }];
    }
    groups.filter(group => group.parts.length).forEach((group, index) => {
      if (index > 0) push(',', 'mark');
      push(group.parts.join(','), group.tone);
    });
  }
  if (quoted) push(quote, 'mark');
  push(raw.slice(raw.trimEnd().length), 'plain');
  return tokens;
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

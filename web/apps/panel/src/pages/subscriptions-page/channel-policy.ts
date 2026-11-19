import type {
  ChannelGroupMember,
  ChannelRemoteRuleSet,
  ChannelRule,
  ChannelRuleGroup,
  SubscriptionChannelConfig,
  SubscriptionFormat,
} from '@/api/api-client';

import { compareText } from '@/utils/compare-text';

/** A group member's identity among the group's candidates, such as `node:<id>` or `builtin:direct`. */
export function memberKey(member: ChannelGroupMember): string {
  return `${member.kind}:${member.id}`;
}

/**
 * Gives a group its members in order. A rule that named a node the group no
 * longer holds routes through the group instead.
 */
export function updateGroupMembers(group: ChannelRuleGroup, members: ChannelGroupMember[]): ChannelRuleGroup {
  const nodes = new Set(members.filter(member => member.kind === 'node').map(member => member.id));
  return {
    ...group,
    members,
    rules: group.rules.map(item => item.exit.kind === 'node' && !nodes.has(item.exit.id!)
      ? { ...item, exit: { kind: 'group-default' } }
      : item),
  };
}

/** A new channel's configuration: no groups and so no nodes, unmatched traffic direct. */
export function emptyChannelConfig(): SubscriptionChannelConfig {
  return { groups: [], default_exit: { kind: 'direct' } };
}

export function newRuleGroup(members: ChannelGroupMember[] = []): ChannelRuleGroup {
  return {
    id: crypto.randomUUID(),
    name: '',
    enabled: true,
    type: 'select',
    members: [...members],
    rules: [],
  };
}
export const defaultGroupHealthCheck = { url: 'https://www.gstatic.com/generate_204', interval: 300, tolerance: 50 };
export function ruleFormats(format: SubscriptionFormat): readonly ChannelRemoteRuleSet['format'][] {
  if (format === 'loon') return ['loon'];
  return format === 'sing-box' ? (['source', 'binary'] as const) : (['yaml', 'text', 'mrs'] as const);
}
export function incompatibleConfig(config: SubscriptionChannelConfig, format: SubscriptionFormat): boolean {
  return (
    (Boolean(config.template) && config.template!.format !== format)
    || config.groups.some((group) =>
      (format === 'sing-box' && (group.type === 'fallback' || group.members.some(member => member.kind === 'builtin' && member.id === 'reject')))
      || (format === 'loon' && group.type !== 'select' && group.members.some(member => member.kind === 'builtin'))
      || group.rules.some(
        (rule) =>
          rule.remote
          && (!ruleFormats(format).includes(rule.remote!.format)
            || (rule.remote.format === 'mrs' && rule.remote.behavior === 'classical')),
      ),
    )
  );
}
const proxyHosts = new Set([
  'mirror.ghproxy.com',
  'gh.api.99988866.xyz',
  'gh-proxy.com',
  'ghproxy.com',
  'ghproxy.net',
  'ghfast.top',
  'hub.gitmirror.com',
]);
export function directRuleURL(raw: string): string {
  let value = raw.trim();
  for (let i = 0; i < 16; i += 1) {
    try {
      const url = new URL(value);
      if (
        url.username
        || url.password
        || !['http:', 'https:'].includes(url.protocol)
        || !(proxyHosts.has(url.hostname) || /\.ghproxy\.(?:com|net)$/.test(url.hostname))
      ) {
        break;
      }
      const slash = value.indexOf('/', value.indexOf('://') + 3);
      const next = slash < 0 ? '' : value.slice(slash + 1);
      if (!/^https?:\/\//.test(next)) break;
      value = next;
    } catch {
      break;
    }
  }
  return value;
}
export function canAccelerateRuleURL(raw: string): boolean {
  try {
    const url = new URL(directRuleURL(raw));
    return (
      ['http:', 'https:'].includes(url.protocol)
      && !url.username
      && !url.password
      && !url.port
      && (url.hostname === 'github.com'
        || url.hostname.endsWith('.github.com')
        || ['raw.githubusercontent.com', 'gist.githubusercontent.com'].includes(url.hostname))
    );
  } catch {
    return false;
  }
}
export function effectiveRuleURL(raw: string, accelerated: boolean): string {
  const direct = directRuleURL(raw);
  return accelerated && canAccelerateRuleURL(direct) ? `https://gh-proxy.com/${direct}` : direct;
}

export function compareChannelRules(left: ChannelRule, right: ChannelRule): number {
  return (left.sort_index ?? 0) - (right.sort_index ?? 0)
    || compareText(left.remote?.name ?? left.value ?? '', right.remote?.name ?? right.value ?? '');
}

export function nextRuleIndex(config: SubscriptionChannelConfig): number {
  const indices = config.groups.flatMap(group => group.rules.map(rule => rule.sort_index ?? 0));
  return indices.length ? Math.min(Number.MAX_SAFE_INTEGER, Math.max(...indices) + 10) : 10;
}

export function detectRuleFormat(raw: string, client: SubscriptionFormat): ChannelRemoteRuleSet['format'] | '' {
  try {
    const url = new URL(directRuleURL(raw));
    if (!['https:', 'http:'].includes(url.protocol)) return '';
    const extension = decodeURIComponent(url.pathname).match(/\.([^/.]+)$/)?.[1].toLowerCase();
    if (client === 'loon') return ['loon', 'list', 'txt'].includes(extension ?? '') ? 'loon' : '';
    const formats: Record<string, ChannelRemoteRuleSet['format']> = client === 'sing-box'
      ? { json: 'source', srs: 'binary' }
      : { yaml: 'yaml', yml: 'yaml', txt: 'text', list: 'text', mrs: 'mrs' };
    return extension && Object.hasOwn(formats, extension) ? formats[extension] : '';
  } catch {
    return '';
  }
}

export function createsGroupCycle(groups: ChannelRuleGroup[], ownerID: string, candidateID: string): boolean {
  const catalog = new Map(groups.map(group => [group.id, group]));
  const visited = new Set<string>();
  function reachesOwner(id: string): boolean {
    if (id === ownerID) return true;
    if (visited.has(id)) return false;
    visited.add(id);
    return (catalog.get(id)?.members ?? []).some(member => member.kind === 'group' && reachesOwner(member.id));
  }
  return reachesOwner(candidateID);
}

/** Removes a group from every other group's members. */
export function removeGroupReferences(groups: ChannelRuleGroup[], id: string): ChannelRuleGroup[] {
  return groups.map(group => group.members.some(member => member.kind === 'group' && member.id === id)
    ? updateGroupMembers(group, group.members.filter(member => !(member.kind === 'group' && member.id === id)))
    : group);
}

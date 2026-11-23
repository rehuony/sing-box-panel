import type {
  ChannelGroupMatch,
  ChannelGroupMember,
  ChannelRemoteRuleSet,
  ChannelRoute,
  ChannelRouteExit,
  ChannelRuleGroup,
  SubscriptionChannelConfig,
  SubscriptionFormat,
  SubscriptionNode,
  SubscriptionRule,
  SubscriptionRuleSet,
} from '@/api/api-client';

import { originFilter } from './subscription-node-pool';

/** A group member's identity among the group's candidates, such as `node:<id>` or `builtin:direct`. */
export function memberKey(member: ChannelGroupMember): string {
  return `${member.kind}:${member.id}`;
}

/** A new channel's configuration: no groups and so no nodes, no exit rules, all traffic direct. */
export function emptyChannelConfig(): SubscriptionChannelConfig {
  return { groups: [], routes: [], default_exit: { kind: 'direct' } };
}

export function newRuleGroup(members: ChannelGroupMember[] = []): ChannelRuleGroup {
  return {
    id: crypto.randomUUID(),
    name: '',
    enabled: true,
    type: 'select',
    members: [...members],
  };
}

/** Whether an exit leaves through the group. */
export function exitsThrough(exit: ChannelRouteExit, groupID: string): boolean {
  return exit.kind === 'group' && exit.id === groupID;
}

/** Whether an exit rule or the final exit leaves through the group, which then cannot be deleted or disabled. */
export function groupInUse(config: SubscriptionChannelConfig, groupID: string): boolean {
  return exitsThrough(config.default_exit, groupID) || config.routes.some(route => exitsThrough(route.exit, groupID));
}

/** Group types as sing-box writes them; mihomo and loon write the stored type itself. */
const singBoxGroupTypes: Partial<Record<ChannelRuleGroup['type'], string>> = { 'select': 'selector', 'url-test': 'urltest' };

/** A group type as the client's configuration writes it, the name the interface gives it untranslated. */
export function groupTypeName(type: ChannelRuleGroup['type'], format: SubscriptionFormat): string {
  return (format === 'sing-box' ? singBoxGroupTypes[type] : undefined) ?? type;
}

export const defaultGroupHealthCheck = { url: 'https://www.gstatic.com/generate_204', interval: 300, tolerance: 50 };
export function ruleFormats(format: SubscriptionFormat): readonly ChannelRemoteRuleSet['format'][] {
  if (format === 'loon') return ['loon'];
  return format === 'sing-box' ? (['source', 'binary'] as const) : (['yaml', 'text', 'mrs'] as const);
}
/** Whether the client cannot use a remote rule set: its format is another client's, or mrs with classical. */
export function unusableRemote(rule: SubscriptionRule | undefined, format: SubscriptionFormat): boolean {
  const remote = rule?.remote;
  return remote !== undefined
    && (!ruleFormats(format).includes(remote.format) || (remote.format === 'mrs' && remote.behavior === 'classical'));
}

/** The rule set a route matches by, when it exists and belongs to the channel's client. */
export function routeRuleSet(
  route: ChannelRoute,
  ruleSets: readonly SubscriptionRuleSet[] | undefined,
  format: SubscriptionFormat,
): SubscriptionRuleSet | undefined {
  const set = route.rule_set_id === undefined ? undefined : ruleSets?.find(value => value.id === route.rule_set_id);
  return set?.client === format ? set : undefined;
}

/**
 * Whether the configuration holds something the client cannot publish, which
 * blocks saving: another client's template, a strategy or built-in the
 * client lacks, an exit rule whose rule set is missing or another client's,
 * or an own remote rule set the client cannot use. Rule sets not yet read
 * flag nothing.
 */
export function incompatibleConfig(
  config: SubscriptionChannelConfig,
  format: SubscriptionFormat,
  ruleSets: readonly SubscriptionRuleSet[] | undefined,
): boolean {
  return (
    (Boolean(config.template) && config.template!.format !== format)
    || config.groups.some(group =>
      (format === 'sing-box' && (group.type === 'fallback' || group.members.some(member => member.kind === 'builtin' && member.id === 'reject')))
      || (format === 'loon' && group.type !== 'select' && group.members.some(member => member.kind === 'builtin')))
    || config.routes.some(route => route.rule_set_id === undefined
      ? unusableRemote(route.rule, format)
      : ruleSets !== undefined && !routeRuleSet(route, ruleSets, format))
  );
}

/**
 * Points each exit rule at the client's rule set of the same name, ignoring
 * case, when the client has one; the others keep the set they name.
 */
export function remapRuleSets(
  routes: ChannelRoute[],
  ruleSets: readonly SubscriptionRuleSet[],
  client: SubscriptionFormat,
): ChannelRoute[] {
  const byID = new Map(ruleSets.map(set => [set.id, set]));
  return routes.map((route) => {
    const current = route.rule_set_id === undefined ? undefined : byID.get(route.rule_set_id);
    if (!current || current.client === client) return route;
    const name = current.name.toLowerCase();
    const next = ruleSets.find(set => set.client === client && set.name.toLowerCase() === name);
    return next ? { ...route, rule_set_id: next.id } : route;
  });
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
    ? { ...group, members: group.members.filter(member => !(member.kind === 'group' && member.id === id)) }
    : group);
}

/** Whether a match names nothing, and so takes no nodes. */
export function emptyMatch(match: ChannelGroupMatch | undefined): boolean {
  return !match || (!match.include.length && !match.exclude.length && !match.origins.length && !match.protocols.length);
}

/**
 * A name rule as a test: text in slashes as a regular expression, which is
 * left out when it does not compile, and other text as a substring in any case.
 */
function nameTest(rule: string): ((name: string) => boolean) | null {
  if (rule.length >= 2 && rule.startsWith('/') && rule.endsWith('/')) {
    try {
      const pattern = new RegExp(rule.slice(1, -1));
      return name => pattern.test(name);
    } catch {
      return null;
    }
  }
  const text = rule.toLowerCase();
  return name => name.toLowerCase().includes(text);
}

/**
 * The pool nodes a group's match takes besides its members, in pool order:
 * from one of its origins and of one of its protocols when it names any,
 * whose display name the include rules keep and the exclude rules leave.
 */
export function matchedNodes(group: ChannelRuleGroup, nodes: readonly SubscriptionNode[]): SubscriptionNode[] {
  const { match } = group;
  if (!match || emptyMatch(match)) return [];
  const tests = (rules: string[]) => rules.flatMap(rule => nameTest(rule) ?? []);
  const include = tests(match.include);
  const exclude = tests(match.exclude);
  const members = new Set(group.members.flatMap(member => member.kind === 'node' ? [member.id] : []));
  return nodes.filter(node => !members.has(node.id)
    && (!match.origins.length || match.origins.includes(originFilter(node)))
    && (!match.protocols.length || match.protocols.includes(node.protocol))
    && (!include.length || include.some(test => test(node.display_name)))
    && !exclude.some(test => test(node.display_name)));
}

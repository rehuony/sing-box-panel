import type {
  ChannelPolicy,
  ChannelRemoteRuleSet,
  ChannelRouteExit,
  ChannelRuleGroup,
  SubscriptionChannel,
  SubscriptionFormat,
  SubscriptionNodeSummary,
} from '@/api/api-client';

import { candidateOrder } from './channel-node-order';

export function updateGroupCandidates(
  group: ChannelRuleGroup,
  node_ids: string[],
  builtin_nodes = group.builtin_nodes,
  candidate_order = candidateOrder(group),
): ChannelRuleGroup {
  const default_exit: ChannelRouteExit = group.default_exit.kind === 'node' && node_ids.includes(group.default_exit.id!)
    ? group.default_exit
    : group.default_exit.kind === 'direct' && builtin_nodes.includes('direct')
      ? group.default_exit
      : node_ids.length
        ? { kind: 'node', id: node_ids[0] }
        : { kind: builtin_nodes[0] ?? 'reject' };
  return {
    ...group, node_ids, builtin_nodes,
    candidate_order: candidateOrder({ node_ids, builtin_nodes, candidate_order }),
    default_exit,
    rules: group.rules.map(item => item.exit.kind === 'node' && !node_ids.includes(item.exit.id!)
      ? { ...item, exit: { kind: 'group-default' } }
      : item),
  };
}

export function initialChannelPolicy(
  channel: Pick<SubscriptionChannel, 'config'>,
  nodes: SubscriptionNodeSummary[],
): ChannelPolicy {
  if (channel.config.policy) return structuredClone(channel.config.policy);
  return {
    selection: {
      ids: nodes
        .filter(
          (node) =>
            node.available
            && !node.hidden
            && !channel.config.exclude_tags?.includes(node.tag)
            && !channel.config.exclude_types?.includes(node.type),
        )
        .map((node) => node.id),
      excluded_ids: [],
      new_node_policy: 'include',
    },
    organizer: { prefix: '', exclude_names: [], sort: 'none', deduplicate: false, incompatible: 'skip' },
    groups: [],
    default_exit: { kind: 'direct' },
  };
}
export function selectedNodeIDs(policy: ChannelPolicy, nodes: SubscriptionNodeSummary[]): Set<string> {
  return new Set([
    ...policy.selection.ids,
    ...nodes
      .filter(
        (node) =>
          policy.selection.new_node_policy === 'include' && !policy.selection.excluded_ids.includes(node.id),
      )
      .map((node) => node.id),
  ]);
}
export function newRuleGroup(nodeIDs: string[]): ChannelRuleGroup {
  return {
    id: crypto.randomUUID(),
    name: '',
    enabled: true,
    type: 'select',
    builtin_nodes: [],
    node_ids: [...nodeIDs],
    default_exit: nodeIDs.length ? { kind: 'node', id: nodeIDs[0] } : { kind: 'reject' },
    rules: [],
  };
}
export const defaultGroupHealthCheck = { url: 'https://www.gstatic.com/generate_204', interval: 300, tolerance: 50 };
export function ruleFormats(format: SubscriptionFormat): readonly ChannelRemoteRuleSet['format'][] {
  if (format === 'loon') return ['loon'];
  return format === 'sing-box' ? (['source', 'binary'] as const) : (['yaml', 'text', 'mrs'] as const);
}
export function incompatiblePolicy(policy: ChannelPolicy, format: SubscriptionFormat): boolean {
  return (
    (Boolean(policy.template) && policy.template!.format !== format)
    || policy.groups.some((group) =>
      (format === 'sing-box' && (group.type === 'fallback' || group.builtin_nodes.includes('reject')))
      || (format === 'loon' && group.type !== 'select' && group.builtin_nodes.length > 0)
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

import type {
  ChannelPolicy,
  ChannelRemoteRuleSet,
  ChannelRule,
  ChannelRuleGroup,
  SubscriptionChannel,
  SubscriptionFormat,
  SubscriptionNodeSummary,
} from '@/api/api-client';

import { compareText } from '@/utils/compare-text';

import { candidateOrder } from './channel-node-order';

export function updateGroupCandidates(
  group: ChannelRuleGroup,
  node_ids: string[],
  builtin_nodes = group.builtin_nodes,
  candidate_order = candidateOrder(group),
): ChannelRuleGroup {
  return {
    ...group, node_ids, builtin_nodes,
    candidate_order: candidateOrder({ node_ids, builtin_nodes, candidate_order }),
    rules: group.rules.map(item => item.exit.kind === 'node' && !node_ids.includes(item.exit.id!)
      ? { ...item, exit: { kind: 'group-default' } }
      : item),
  };
}

export function initialChannelPolicy(
  channel: Pick<SubscriptionChannel, 'config'>,
  nodes: SubscriptionNodeSummary[],
): ChannelPolicy {
  if (channel.config.policy) {
    const policy = structuredClone(channel.config.policy);
    let index = 0;
    policy.groups.forEach(group => group.rules.forEach(rule => {
      index += 10;
      rule.sort_index ??= index;
    }));
    return policy;
  }
  const excluded = nodes.filter(node => channel.config.exclude_tags?.includes(node.tag)
    || channel.config.exclude_types?.includes(node.type)).map(node => node.id);
  return {
    selection: {
      ids: nodes
        .filter(
          (node) =>
            node.available
            && !node.hidden
            && !excluded.includes(node.id),
        )
        .map((node) => node.id),
      excluded_ids: excluded,
      new_node_policy: 'include',
    },
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
    candidate_order: nodeIDs.map(id => `node:${id}`),
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

export function compareChannelRules(left: ChannelRule, right: ChannelRule): number {
  return (left.sort_index ?? 0) - (right.sort_index ?? 0)
    || compareText(left.remote?.name ?? left.value ?? '', right.remote?.name ?? right.value ?? '');
}

export function nextRuleIndex(policy: ChannelPolicy): number {
  const indices = policy.groups.flatMap(group => group.rules.map(rule => rule.sort_index ?? 0));
  return Math.min(2147483647, Math.max(0, ...indices) + 10);
}

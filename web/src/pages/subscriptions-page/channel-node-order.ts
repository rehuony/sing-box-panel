import type { ChannelRuleGroup } from '@/api/api-client';

export function candidateOrder(group: Pick<ChannelRuleGroup, 'node_ids' | 'builtin_nodes' | 'candidate_order'>): string[] {
  const members = [...group.node_ids.map((id) => `node:${id}`), ...group.builtin_nodes.map((kind) => `builtin:${kind}`)];
  const selected = new Set(members);
  return [...new Set([...(group.candidate_order ?? []).filter((id) => selected.has(id)), ...members])];
}

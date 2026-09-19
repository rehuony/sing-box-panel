import type { SubscriptionNodeSummary } from '@/api/api-client';

export function subscriptionNodeAddress(node: SubscriptionNodeSummary): string {
  if (node.realm_url) return node.realm_url;
  if (!node.server) return '';
  const host
    = node.server.includes(':') && !node.server.startsWith('[') ? `[${node.server}]` : node.server;
  return `${host}:${node.server_ports?.length ? node.server_ports.join(', ') : node.port}`;
}

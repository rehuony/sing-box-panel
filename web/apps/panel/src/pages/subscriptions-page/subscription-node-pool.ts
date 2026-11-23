import type { SubscriptionNode } from '@/api/api-client';

/** The origin filter's value for every node. */
export const ALL_ORIGINS = 'all';
/** The origin filter's value for the inbounds the fleet's nodes publish. */
export const FLEET_ORIGIN = 'fleet';
/** The protocol filter's value for every protocol. */
export const ALL_PROTOCOLS = '';

/** Where a node comes from: its fleet node and inbound, or its source. */
export function nodeOrigin(node: SubscriptionNode): string {
  const { origin } = node;
  if (origin.kind === 'source') return origin.source_name ?? '';
  return [origin.node_name, origin.inbound_tag].filter(Boolean).join(' · ');
}

/** The origin filter value a node matches besides every origin. */
export function originFilter(node: SubscriptionNode): string {
  return node.origin.kind === 'source' ? `source:${node.origin.source_id ?? ''}` : FLEET_ORIGIN;
}

/** Where a client connects: a realm URL, or the server with its port or port ranges. */
export function nodeAddress(node: SubscriptionNode): string {
  if (node.realm_url) return node.realm_url;
  if (!node.server) return '';
  const host = node.server.includes(':') && !node.server.startsWith('[') ? `[${node.server}]` : node.server;
  const ports = node.server_ports?.length ? node.server_ports.join(',') : node.port;
  return ports === undefined ? host : `${host}:${ports}`;
}

/** The node's security layer: tls and reality. */
export function nodeSecurity(node: SubscriptionNode): string[] {
  return [node.tls ? 'tls' : '', node.reality ? 'reality' : ''].filter(Boolean);
}

/** Whether a search, already lower case, matches the node's display name, origin, protocol or server. */
export function matchesNode(node: SubscriptionNode, query: string): boolean {
  if (query === '') return true;
  return [node.display_name, nodeOrigin(node), node.protocol, node.server ?? '', node.realm_url ?? '']
    .some(value => value.toLowerCase().includes(query));
}

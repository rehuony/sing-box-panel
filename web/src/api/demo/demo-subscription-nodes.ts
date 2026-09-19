import type { DemoData } from './demo-data';
import type { ApiClient, SubscriptionNodeDetail, SubscriptionNodeSummary } from '../api-client';

import { ApiRequestError } from '../api-client';

function details(id: string, raw: string, previous?: SubscriptionNodeDetail): SubscriptionNodeDetail {
  const value = JSON.parse(raw);
  if (!value || typeof value !== 'object' || Array.isArray(value) || !value.type || !value.tag || !value.server
    || !Number.isInteger(value.server_port) || value.server_port < 1 || value.server_port > 65535) {
    throw new ApiRequestError('Invalid node.', { status: 422, code: 'subscription_invalid' });
  }
  return {
    id, key: `manual:${id}`, name: value.tag, tag: value.tag, source_id: 'manual', source_name: '手动节点',
    origin: 'manual', type: value.type, available: true, hidden: previous?.hidden ?? false,
    visibility_revision: previous?.visibility_revision ?? 0, revision: (previous?.revision ?? 0) + 1,
    server: value.server, port: value.server_port, tls: value.tls?.enabled === true,
    reality: value.tls?.reality?.enabled === true, sni: value.tls?.server_name, outbound_json: raw,
  };
}

export function nodeSummary({ outbound_json: _outbound, ...node }: SubscriptionNodeDetail): SubscriptionNodeSummary {
  return node;
}

export function createDemoNodeApi(initial: SubscriptionNodeDetail[]) {
  let nodes = structuredClone(initial);
  const find = (id: string) => {
    const node = nodes.find(value => value.id === id);
    if (!node) throw new ApiRequestError('Node not found.', { status: 404, code: 'subscription_resource_not_found' });
    return node;
  };
  const editable = (id: string, revision: number) => {
    const node = find(id);
    if (node.origin !== 'manual') throw new ApiRequestError('Manage this node at its source.', { status: 422, code: 'subscription_invalid' });
    if (revision !== node.revision) throw new ApiRequestError('Node changed.', { status: 412, code: 'subscription_version_conflict' });
    return node;
  };
  return {
    getSubscriptionNodeCatalog: async () => ({ applied_bundle_id: 'bundle_demo_current', nodes: nodes.map(nodeSummary), diagnostics: [] }),
    getSubscriptionNode: async id => structuredClone(find(id)),
    createSubscriptionNode: async raw => {
      const node = details(`node_demo_${crypto.randomUUID()}`, raw);
      nodes.push(node);
      return structuredClone(node);
    },
    updateSubscriptionNode: async (id, raw, revision) => {
      const previous = editable(id, revision);
      const node = details(id, raw, previous);
      nodes = nodes.map(value => value.id === id ? node : value);
      return structuredClone(node);
    },
    deleteSubscriptionNode: async (id, revision) => {
      editable(id, revision);
      nodes = nodes.filter(value => value.id !== id);
    },
    setSubscriptionNodeVisibility: async (id, hidden, revision) => {
      const node = find(id);
      if (revision !== node.visibility_revision) throw new ApiRequestError('Visibility changed.', { status: 412, code: 'subscription_version_conflict' });
      node.hidden = hidden;
      node.visibility_revision += 1;
      return structuredClone(nodeSummary(node));
    },
    parseSubscriptionNode: async text => {
      // Demo accepts native JSON; production also uses the server's URI parser.
      details('preview', text);
      return { outbound_json: text };
    },
  } satisfies Pick<ApiClient, 'getSubscriptionNodeCatalog' | 'getSubscriptionNode' | 'createSubscriptionNode'
  | 'updateSubscriptionNode' | 'deleteSubscriptionNode' | 'setSubscriptionNodeVisibility' | 'parseSubscriptionNode'>;
}

export function demoManualNodes(): SubscriptionNodeDetail[] {
  return [
    { ...details('node_demo_local', '{"type":"socks","tag":"mixed-in","server":"203.0.113.10","server_port":2080}'),
      origin: 'local', source_id: 'local', revision: undefined, listener: '127.0.0.1:2080' },
    details('node_demo_manual', '{"type":"vless","tag":"香港","server":"hk.example.com","server_port":443,"uuid":"00000000-0000-4000-8000-000000000001","tls":{"enabled":true,"server_name":"hk.example.com"}}'),
  ];
}

export function demoSourceNodeDetails(data: DemoData): SubscriptionNodeDetail[] {
  return data.sources.flatMap(source => (data.sourceVersions.get(source.id)?.[0]?.normalized_nodes ?? [])
    .filter(value => typeof value.server === 'string' && typeof value.server_port === 'number')
    .map((value, index) => ({
      ...details(`node_${source.id}_${index}`, JSON.stringify(value)), origin: 'source' as const,
      key: `${source.id}:${index}`, source_id: source.id, source_name: source.name, available: source.enabled, revision: undefined,
    })));
}

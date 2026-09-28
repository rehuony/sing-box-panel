import type { DemoData } from './demo-data';
import type { ApiClient, SubscriptionNodeDetail, SubscriptionNodeOrder, SubscriptionNodeSummary, SubscriptionSourceSummary } from '../api-client';

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

export function createDemoNodeApi(initial: SubscriptionNodeDetail[], getSources: () => Pick<SubscriptionSourceSummary, 'id' | 'name' | 'enabled' | 'source_kind' | 'created_at'>[]) {
  let nodes = structuredClone(initial);
  const orders: Record<string, SubscriptionNodeOrder> = {};
  const catalog = () => {
    const sources = new Map(getSources().map(source => [source.id, source]));
    const remoteSources = [...sources.values()].filter(source => source.source_kind === 'remote').toSorted((a, b) => b.created_at.localeCompare(a.created_at) || b.id.localeCompare(a.id));
    const collections = new Map(remoteSources.map((source, index) => [source.id, index + 1]));
    const collection = (node: SubscriptionNodeSummary) => collections.has(node.source_id) ? node.source_id : 'manual';
    nodes = nodes.filter(node => node.origin !== 'source' || sources.has(node.source_id));
    for (const node of nodes) {
      const source = sources.get(node.source_id);
      if (node.origin === 'source' && source) {
        node.source_name = source.source_kind === 'local' ? '手动节点' : source.name;
        node.available = source.enabled;
      }
    }
    for (const id of Object.keys(orders)) {
      if (id !== 'manual' && !sources.has(id)) delete orders[id];
    }
    const positions = new Map(Object.entries(orders)
      .map(([id, order]) => [id, new Map(order.ids.map((nodeID, index) => [nodeID, index]))]));
    const position = (node: SubscriptionNodeSummary) => positions.get(collection(node))?.get(node.id) ?? Infinity;
    return {
      collections, collection,
      nodes: nodes.toSorted((a, b) => (collections.get(a.source_id) ?? 0) - (collections.get(b.source_id) ?? 0)
        || position(a) - position(b)),
    };
  };
  const find = (id: string) => {
    const node = catalog().nodes.find(value => value.id === id);
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
    getSubscriptionNodeCatalog: async () => ({ applied_bundle_id: 'bundle_demo_current', nodes: catalog().nodes.map(nodeSummary), diagnostics: [], node_orders: structuredClone(orders) }),
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
    setSubscriptionNodeOrder: async (collectionID, ids, revision) => {
      const current = catalog();
      const members = new Set(current.nodes
        .filter(node => current.collection(node) === collectionID)
        .map(node => node.id));
      if ((collectionID !== 'manual' && !current.collections.has(collectionID))
        || ids.length > 10_000 || new Set(ids).size !== ids.length
        || ids.some(id => !members.has(id))) {
        throw new ApiRequestError('Invalid order.', { status: 422, code: 'subscription_invalid' });
      }
      if (revision !== (orders[collectionID]?.revision ?? 0)) throw new ApiRequestError('Order changed.', { status: 412, code: 'subscription_version_conflict' });
      orders[collectionID] = { ids: [...ids], revision: revision + 1 };
      return structuredClone(orders[collectionID]);
    },
    parseSubscriptionNode: async text => {
      // Demo accepts native JSON; production also uses the server's URI parser.
      details('preview', text);
      return { outbound_json: text };
    },
  } satisfies Pick<ApiClient, 'getSubscriptionNodeCatalog' | 'getSubscriptionNode' | 'createSubscriptionNode'
  | 'updateSubscriptionNode' | 'deleteSubscriptionNode' | 'setSubscriptionNodeVisibility' | 'setSubscriptionNodeOrder' | 'parseSubscriptionNode'>;
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

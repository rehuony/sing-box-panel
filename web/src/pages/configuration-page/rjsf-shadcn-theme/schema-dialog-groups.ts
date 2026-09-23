import type { RJSFSchema } from '@rjsf/utils';

import { resolvedSchema, schemaProperties } from '../schema-ui';

const connectionFields = new Set([
  'detour', 'domain_resolver', 'domain_strategy', 'bind_address_no_port', 'bind_interface',
  'connect_timeout', 'disable_tcp_keep_alive', 'fallback_delay', 'fallback_network_type',
  'inet4_bind_address', 'inet6_bind_address', 'netns', 'network_strategy', 'network_type',
  'protect_path', 'reuse_addr', 'routing_mark', 'tcp_fast_open', 'tcp_keep_alive',
  'tcp_keep_alive_interval', 'tcp_multi_path', 'udp_fragment',
]);
const commonMatchFields = new Set([
  'type', 'inbound', 'domain', 'domain_suffix', 'ip_cidr', 'ip_is_private', 'network',
  'protocol', 'port', 'rule_set', 'invert', 'mode', 'rules',
]);
const basicFields = new Set([
  'type', 'tag', 'name', 'server', 'server_port', 'server_ports', 'listen', 'listen_port',
  'address', 'interface', 'interface_name', 'path', 'url', 'format', 'method', 'version',
  'outbounds', 'default', 'interrupt_exist_connections',
]);
const authenticationFields = new Set(['users', 'username', 'password', 'uuid', 'auth', 'auth_str', 'private_key', 'private_key_path', 'private_key_passphrase', 'public_key', 'pre_shared_key']);
const transportFields = new Set(['transport', 'multiplex', 'obfs', 'udp_over_tcp']);

export type SchemaDialogGroup = 'basic' | 'match' | 'more' | 'action' | 'authentication' | 'tls' | 'transport' | 'connection' | 'advanced';

/** Discover action fields from the schema itself, including version-specific actions. */
function actionFields(
  schema: RJSFSchema, root: RJSFSchema, result = new Set<string>(), visited = new Set<RJSFSchema>(),
): Set<string> {
  const value = resolvedSchema(schema, root);
  if (visited.has(value)) return result;
  visited.add(value);
  if (value.properties?.action) {
    for (const key of Object.keys(value.properties)) result.add(key);
  }
  for (const branch of [...(value.allOf ?? []), ...(value.oneOf ?? []), ...(value.anyOf ?? [])]) {
    if (typeof branch === 'object') actionFields(branch, root, result, visited);
  }
  return result;
}

export function schemaDialogGroups(schema: RJSFSchema, root: RJSFSchema, data: unknown) {
  // Resolve the selected protocol while retaining any not-yet-selected action alternatives.
  const selected = resolvedSchema(schema, root, data);
  const properties = schemaProperties(selected, root);
  const compact = Object.keys(properties).length < 6
    && Object.values(properties).every((property) => {
      const field = resolvedSchema(property, root);
      return field.type !== 'object' && field.type !== 'array' && !field.oneOf && !field.anyOf;
    });
  const actions = actionFields(schema, root);
  const rule = 'action' in properties && ('domain' in properties || 'rules' in properties);
  const primary: SchemaDialogGroup = rule && !compact ? 'match' : 'basic';
  function groupForField(key: string): SchemaDialogGroup {
    if (compact) return 'basic';
    if (rule) {
      if (commonMatchFields.has(key)) return 'match';
      if (actions.has(key)) return 'action';
      return 'more';
    }
    if (key === 'tls') return 'tls';
    if (transportFields.has(key)) return 'transport';
    if (authenticationFields.has(key)) return 'authentication';
    if (connectionFields.has(key)) return 'connection';
    if (Object.keys(properties).length > 12 && !basicFields.has(key)) return 'advanced';
    return 'basic';
  }
  const present = new Set(Object.keys(properties).map(groupForField));
  const order: SchemaDialogGroup[] = [primary, 'more', 'action', 'authentication', 'tls', 'transport', 'connection', 'advanced'];
  const groups = order.filter((group) => group === primary || present.has(group));
  return { primary, groupForField, groups, compact };
}

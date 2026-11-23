import type { RJSFSchema } from '@rjsf/utils';

import type { FilesystemMode } from '@/api/api-client';

const fileFields = new Set([
  'certificate_path', 'certificate_authority_path', 'client_certificate_path', 'client_key_path',
  'key_path', 'private_key_path', 'static_key_path', 'mca_certificate_path', 'mca_key_path',
  'crl_path', 'credential_path', 'secret_path', 'mesh_psk_file', 'initial_path',
  'executable_path', 'wrapper_path', 'process_path', 'dhcp_lease_files',
]);
const directoryFields = new Set(['certificate_directory_path', 'data_directory', 'state_directory', 'taildrop_directory', 'external_ui']);
const outputFields = new Set(['cache_path', 'usages_path', 'pid_file']);

// Classification is presentation-only. Ambiguous names must be paired with
// their schema definition / document context and protocol discriminator.
export function configurationPathMode(key: string, context: readonly string[]): FilesystemMode | undefined {
  const scope = context.map(part => part.toLowerCase());
  const has = (name: string) => scope.some(part => part.includes(name));
  if (fileFields.has(key)) return 'file';
  if (directoryFields.has(key)) return 'directory';
  if (outputFields.has(key)) return 'output-file';
  if (key === 'protect_path') return 'socket';
  if (key === 'config_path') return has('derp') ? 'output-file' : 'file';
  if (key === 'directory' && (has('masquerade') || has('hysteria2'))) return 'directory';
  if (key === 'dashboard' && has('service') && scope.includes('api')) return 'directory';
  if (key === 'output' && (scope.includes('log') || scope.includes('logoptions'))) return 'output-file';
  if (key === 'cache_file' && (has('clashapi') || has('clash_api'))) return 'output-file';
  if (key === 'path') {
    if (has('cachefile') || has('cache_file')) return 'output-file';
    if (has('dashboard') && scope.includes('api')) return 'directory';
    if (has('networknamespace') || has('network_namespace')) return 'file';
    if ((has('ruleset') || has('rule_set')) && scope.includes('local')) return 'file';
    if (has('dns') && scope.includes('hosts')) return 'file';
  }
  return undefined;
}

export function readConfigurationPathMode(schema: RJSFSchema): FilesystemMode | undefined {
  return schema['x-panel-path'] as FilesystemMode | undefined;
}

// Carry a property's mode through scalar/list representations without tagging
// unrelated object children or mutating a referenced shared string definition.
export function withConfigurationPathMode(schema: RJSFSchema, mode: FilesystemMode | undefined): RJSFSchema {
  if (!mode) return schema;
  const result = { ...schema, 'x-panel-path': mode };
  for (const keyword of ['anyOf', 'oneOf'] as const) {
    if (schema[keyword]) result[keyword] = schema[keyword].map(branch => typeof branch === 'boolean' ? branch : withConfigurationPathMode(branch, mode));
  }
  if (schema.items && typeof schema.items === 'object' && !Array.isArray(schema.items)) {
    result.items = withConfigurationPathMode(schema.items, mode);
  }
  return result;
}

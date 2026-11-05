import type { RJSFSchema } from '@rjsf/utils';

type CredentialKind = 'password' | 'uuid' | 'base64' | 'shadowsocks';
export type CredentialGenerator = 'password' | 'uuid' | 'base64-16' | 'base64-32';

export function configurationCredentialKind(key: string, context: readonly string[]): CredentialKind | undefined {
  const has = (name: string) => context.some(part => part.toLowerCase().includes(name));
  if (key.toLowerCase() === 'password') return has('shadowsocks') ? 'shadowsocks' : 'password';
  if (['private_key_passphrase', 'client_key_password', 'mca_key_password'].includes(key)) return 'password';
  if (key === 'uuid') return 'uuid';
  // 24 ASCII characters also fit Snell v6's 12–255 byte PSK requirement.
  if ((key === 'psk' || key === 'userkey') && has('snell')) return 'password';
  if (key === 'pre_shared_key' && has('wireguard')) return 'base64';
  if (has('hysteria') && !has('hysteria2')) {
    if (key === 'auth') return 'base64';
    if (key === 'auth_str' || key === 'obfs') return 'password';
  }
  if (key === 'secret' && (has('clashapi') || has('clash_api'))) return 'password';
  return undefined;
}

// Presentation annotations never alter the authoritative schema or stored data.
export function withConfigurationCredential(schema: RJSFSchema, kind: CredentialKind | undefined): RJSFSchema {
  if (!kind) return schema;
  // Nested dialogs may receive an already resolved definition without its name.
  // Retain its specific protocol annotation when annotating that schema again.
  const result = { ...schema, 'x-panel-credential': schema['x-panel-credential'] ?? kind };
  for (const keyword of ['anyOf', 'oneOf'] as const) {
    if (schema[keyword]) result[keyword] = schema[keyword].map(branch => typeof branch === 'boolean' ? branch : withConfigurationCredential(branch, kind));
  }
  return result;
}

export function credentialGenerator(schema: RJSFSchema, method?: string): CredentialGenerator | undefined {
  if (schema.type !== 'string') return undefined;
  const kind = schema['x-panel-credential'] as CredentialKind | undefined;
  if (kind === 'base64') return 'base64-32';
  if (kind !== 'shadowsocks') return kind;
  // sing-box's Shadowsocks 2022 passwords encode raw key bytes, not a string
  // truncated to the key size. An unset/unknown method must not guess a format.
  switch (method) {
    case '2022-blake3-aes-128-gcm': return 'base64-16';
    case '2022-blake3-aes-256-gcm':
    case '2022-blake3-chacha20-poly1305': return 'base64-32';
    case 'aes-128-gcm':
    case 'aes-192-gcm':
    case 'aes-256-gcm':
    case 'chacha20-ietf-poly1305':
    case 'xchacha20-ietf-poly1305':
    case 'aes-128-ctr':
    case 'aes-192-ctr':
    case 'aes-256-ctr':
    case 'aes-128-cfb':
    case 'aes-192-cfb':
    case 'aes-256-cfb':
    case 'rc4-md5':
    case 'chacha20-ietf':
    case 'xchacha20': return 'password';
    default: return undefined;
  }
}

export function generateCredential(generator: CredentialGenerator): string {
  const length = generator === 'base64-32' ? 32 : generator === 'password' ? 18 : 16;
  const bytes = crypto.getRandomValues(new Uint8Array(length));
  if (generator === 'uuid') {
    // UUID v4 using getRandomValues also works on HTTP panel origins, where
    // crypto.randomUUID is unavailable (it requires a secure context).
    bytes[6] = (bytes[6] & 0x0F) | 0x40;
    bytes[8] = (bytes[8] & 0x3F) | 0x80;
    const hex = Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('');
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
  }
  const base64 = btoa(String.fromCharCode(...bytes));
  return generator === 'password' ? base64.replaceAll('+', '-').replaceAll('/', '_') : base64;
}

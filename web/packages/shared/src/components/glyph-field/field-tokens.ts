/**
 * Text the console field draws: names, settings and fragments from a proxy
 * node's work. Addresses and hosts come from the documentation ranges and
 * example domains, so the field never shows a real endpoint.
 */

/** How far a glyph sits: 0 is the far layer, 2 the near one. */
export type FieldDepth = 0 | 1 | 2;

const SHORT = [
  'tcp', 'udp', 'tls', 'quic', 'ws', 'grpc', 'h2', 'h3', 'http', 'ssh', 'dns', 'mux', 'tun', 'socks',
  'direct', 'block', 'vless', 'vmess', 'trojan', 'hy2', 'tuic', 'reality', 'utls', 'sniff', 'alpn', 'bbr',
  'SYN', 'ACK', 'RTT', 'ok', '200', ':22', ':53', ':80', ':443', ':853', ':8080', ':8443', '::1',
] as const;

const MEDIUM = [
  'inbounds', 'outbounds', 'route', 'rules', 'rule_set', 'listen_port', 'server_name', 'tcp_fast_open',
  'udp_over_tcp', 'auto_route', 'stack: system', 'x25519', 'tls1.3', 'h2,http/1.1', 'chacha20-poly1305',
  'aes-128-gcm', 'aes-256-gcm', '0.0.0.0/0', '::/0', '"tag": "proxy"', '"type": "vless"', '"final": "direct"',
  '--config', 'check -c', 'cdn.example.com', 'www.example.org', 'api.example.net', 'fingerprint: chrome',
  'min_version', 'keepalive 15s', 'mtu 1500', 'padding', 'handshake', 'session ticket',
] as const;

const LONG = [
  '2022-blake3-aes-128-gcm', '"listen": "::", "listen_port": 443', 'route.final = "proxy"',
  'inbound/vless[in]: accepted', 'outbound/direct: dial 198.51.100.7:443', 'tls: handshake done in 41ms',
  '"flow": "xtls-rprx-vision"', 'sing-box run -c config.json', 'dns: exchanged A cdn.example.com',
  '"sniff_override_destination": false', 'router: match rule_set => direct', 'connection closed: EOF',
  'quic: idle timeout after 30s', '"server": "203.0.113.42"', 'udp: forwarded 1350 bytes',
] as const;

function pick<T>(items: readonly T[]): T {
  return items[Math.floor(Math.random() * items.length)]!;
}

function hex(length: number): string {
  let value = '';
  while (value.length < length) value += Math.floor(Math.random() * 16).toString(16);
  return value;
}

function integer(minimum: number, maximum: number): number {
  return minimum + Math.floor(Math.random() * (maximum - minimum + 1));
}

const GENERATED = [
  () => hex(2),
  () => `0x${hex(4)}`,
  () => `:${integer(1024, 65535)}`,
  () => `${hex(8)}-${hex(4)}`,
  () => `${pick(['192.0.2', '198.51.100', '203.0.113'])}.${integer(1, 254)}`,
  () => `2001:db8::${hex(integer(2, 4))}`,
  () => `${(Math.random() * 90 + 0.1).toFixed(1)} ${pick(['KB/s', 'MB/s'])}`,
  () => `${integer(8, 240)}ms`,
] as const;

const NEAR_GENERATED = [GENERATED[0], GENERATED[1], GENERATED[2], GENERATED[7]] as const;

/** Text for a glyph at depth; nearer glyphs stay short, far ones carry fragments. */
export function fieldToken(depth: FieldDepth): string {
  const roll = Math.random();
  if (depth === 2) return roll < 0.55 ? pick(SHORT) : pick(NEAR_GENERATED)();
  if (depth === 1) return roll < 0.3 ? pick(SHORT) : roll < 0.65 ? pick(MEDIUM) : pick(GENERATED)();
  return roll < 0.2 ? pick(LONG) : roll < 0.55 ? pick(MEDIUM) : roll < 0.72 ? pick(SHORT) : pick(GENERATED)();
}

import type { RJSFSchema } from '@rjsf/utils';

import { configurationFieldCopy } from '@/i18n/locales/zh-CN/configuration-fields';

type FieldCopy = readonly [label: string, description: string];

// Context is a schema definition / JSON field path, with union discriminator
// values appended. Never infer the meaning of an ambiguous key from its name alone.
export function configurationFieldHelp(key: string, context: readonly string[]): FieldCopy | undefined {
  const scope = context.join('/').toLowerCase();
  const has = (value: string) => scope.includes(value);
  if (key === 'disabled' && has('log')) return ['禁用日志', '关闭 sing-box 日志输出。'];
  if (key === 'address' && has('wireguardpeer')) return ['对端 IP 地址', 'WireGuard 对等节点的 IP 地址；与对端端口配合使用。'];
  if (key === 'port' && has('wireguardpeer')) return ['对端端口', 'WireGuard 对等节点监听的 UDP 端口。'];
  if (key === 'remote' && has('openvpn')) return ['固定远端地址', 'OpenVPN UDP 静态密钥服务端使用的固定对端地址，需同时指定 remote_port。'];
  if (key === 'final') {
    if (has('dns')) return ['默认 DNS 服务器', 'DNS 规则未指定服务器时使用的服务器标签；留空使用第一个 DNS 服务器。'];
    if (has('route')) return ['默认出站', '路由规则未指定出站时使用的出站标签；留空使用第一个出站。'];
  }
  if (key === 'server' && (has('dnsrule') || has('domainresolver') || has('domain_resolver') || has('resolve') || has('dns/rules'))) {
    return ['DNS 服务器标签', '引用已配置的 DNS 服务器标签；这里填写标签，而不是服务器 IP 或域名。'];
  }
  if (key === 'server' && has('openconnect')) return ['VPN 服务器 URL', 'OpenConnect 服务器的 HTTPS URL；省略协议时使用 https://。'];
  if (key === 'method') {
    if (has('reject')) {
      return ['拒绝方式', has('dns')
        ? 'default 返回 DNS REFUSED，drop 丢弃查询。'
        : 'default 拒绝连接，drop 丢弃数据包；支持时 reply 可回复 ICMP 回显请求。'];
    }
    if (has('shadowsocks') || has('shadowsocksdestination')) return ['加密方法', 'Shadowsocks 加密算法名称；客户端与服务端必须使用相同算法和匹配的密钥。'];
    if (has('transport') || has('http')) return ['HTTP 请求方法', 'HTTP 传输使用的请求方法，例如 GET 或 POST，需与服务端配置匹配。'];
  }
  if (key === 'mode') {
    if (has('openvpn')) return ['VPN 会话模式', 'tls 使用 TLS 控制通道；static_key 为旧式静态密钥模式，不提供前向保密。'];
    if (has('token')) return ['令牌模式', '选择 totp、hotp、stoken 或 oidc，决定令牌的生成与认证方式。'];
    if (has('snell')) return ['流量整形模式', 'Snell v6 使用 default、unshaped 或 unsafe-raw，需与对端要求一致。'];
  }
  if (key === 'address' && (has('tun') || has('endpoint') || has('wireguard'))) {
    return ['接口地址前缀', '虚拟接口的 IPv4 或 IPv6 CIDR 地址，例如 172.19.0.1/30；不是远端服务器地址。'];
  }
  if (key === 'address' && has('legacy')) return ['DNS 服务器地址', '旧版 DNS 上游地址，可包含协议前缀；新配置优先使用独立类型的 DNS 服务器。'];
  if (key === 'interval') {
    if (has('urltest')) return ['测速间隔', 'URLTest 检测候选出站延迟的间隔，例如 3m。'];
    if (has('ntp')) return ['时间同步间隔', '向 NTP 服务器重新同步时间的间隔，例如 30m。'];
  }
  if (key === 'timeout') {
    if (has('optimistic')) return ['过期缓存可用时长', 'DNS 缓存过期后仍可乐观返回的最长时间，例如 3d；不是查询超时。'];
    if (has('sniff')) return ['协议探测超时', '等待识别连接协议的最长时间，例如 300ms。'];
    if (has('dns') || has('resolver') || has('resolve')) return ['DNS 查询超时', '等待 DNS 查询完成的最长时间；规则或解析器中的值可覆盖全局超时。'];
  }
  if (key === 'protocol' && has('multiplex')) return ['复用协议', '选择 smux、yamux 或 h2mux；客户端与服务端需支持相同的复用协议。'];
  if (key === 'type' && (has('rule') && !has('ruleset') && !has('rule_set'))) return ['规则类型', '选择普通匹配规则或 logical 逻辑规则；逻辑规则使用子规则组合条件。'];
  if (key === 'provider' && !has('dns01')) return ['证书颁发机构', 'ACME 使用的证书颁发机构，例如 letsencrypt、zerossl 或自定义服务 URL。'];
  if (key === 'domain' && (has('acme') || has('certificateprovider'))) return ['证书域名', '需要申请或管理证书的域名列表。'];
  if (key === 'name' && (has('endpoint') || has('wireguard'))) return ['接口名称', '端点创建的系统网络接口名称；留空使用自动生成的名称。'];
  if (key === 'auth' && has('openvpn')) return ['数据通道认证摘要', 'OpenVPN 的认证摘要算法；用于非 AEAD 数据通道加密与 tls-auth。'];
  if (key === 'transport' && has('openvpnpushdns')) return ['DNS 传输方式', '推送给客户端的 DNS 传输方式：plain、dot 或 doh。'];
  if (key === 'key' && has('ech')) return ['ECH 密钥', 'PEM 格式的 ECH 服务端密钥；需与客户端使用的 ECH 配置配对。'];
  if (key === 'certificate' && (has('outboundtls') || has('openvpnoutboundtls'))) return ['受信任服务器证书', '用于信任或验证服务器的 PEM 证书内容；也可通过证书文件配置。'];
  if (key === 'client_certificate' && has('inboundtls')) return ['受信任客户端证书', '用于验证客户端身份的 PEM 证书，配合客户端证书验证策略使用。'];
  if (key === 'path') {
    if (has('cachefile') || has('cache_file')) return ['缓存文件路径', '保存缓存数据的文件路径；留空使用 cache.db。'];
    if (has('ruleset') || has('rule_set')) return ['规则集文件路径', '本地规则集文件的位置；内容格式需与所选规则集格式一致。'];
    if (has('transport') || has('https') || has('http3')) return ['请求路径', 'HTTP 请求或传输握手的路径，例如 /dns-query；需与服务端一致。'];
  }
  return Object.hasOwn(configurationFieldCopy, key) ? configurationFieldCopy[key] : undefined;
}

export interface ConfigurationFieldHelp {
  key: string;
  label: string;
  description: string;
}

export function readConfigurationFieldHelp(schema: RJSFSchema): ConfigurationFieldHelp | undefined {
  return schema['x-panel-field-help'] as ConfigurationFieldHelp | undefined;
}

export function withConfigurationFieldHelp(schema: RJSFSchema, key: string, context: readonly string[]): RJSFSchema {
  const copy = configurationFieldHelp(key, context);
  // Array dialogs already carry the exact parent context; do not replace it.
  if (!copy || readConfigurationFieldHelp(schema)) return schema;
  return { ...schema, 'x-panel-field-help': { key, label: copy[0], description: copy[1] } };
}

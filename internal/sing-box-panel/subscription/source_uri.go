// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// shareLinkSourceParser reads share links, one per line. Base64-wrapped
// content is decoded before parsing; blank lines and lines starting with #
// are skipped. With a protocol, every link must convert to it.
type shareLinkSourceParser struct {
	protocol string
}

func (shareLinkSourceParser) Format() SourceFormat { return SourceFormatShareLinks }

var shareLinkScheme = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*)://`)

func (shareLinkSourceParser) Sniff(body []byte) int {
	lines, links := 0, 0
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		lines++
		if shareLinkScheme.Match(line) {
			links++
		}
	}
	if links == 0 {
		return 0
	}
	return 100 * links / lines
}

func (parser shareLinkSourceParser) Items(body []byte) ([]SourceItem, error) {
	items := []SourceItem{}
	for index, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		location := fmt.Sprintf("line %d", index+1)
		match := shareLinkScheme.FindStringSubmatch(line)
		if match == nil {
			return nil, contentError(location, "not a share link")
		}
		scheme := strings.ToLower(match[1])
		converter, known := shareLinkConverters[scheme]
		if parser.protocol != "" && converter.protocol != parser.protocol {
			return nil, contentError(location, "not a "+parser.protocol+" link")
		}
		if !known {
			name := scheme
			if parsed, err := url.Parse(line); err == nil {
				if fragment := linkName(parsed); fragment != "" {
					name = fragment
				} else if port, err := strconv.ParseInt(parsed.Port(), 10, 64); err == nil {
					name = unnamedNode(scheme, parsed.Hostname(), port)
				}
			}
			items = append(items, SourceItem{Location: location, Name: name, Protocol: scheme, convert: func() (map[string]any, error) {
				return nil, errors.New("unsupported share link scheme")
			}})
			continue
		}
		summary, err := converter.describe(line)
		if err != nil {
			return nil, contentError(location, err.Error())
		}
		name := summary.name
		if name == "" {
			name = unnamedNode(converter.protocol, summary.server, summary.port)
		}
		link := line
		items = append(items, SourceItem{Location: location, Name: name, Protocol: converter.protocol, convert: func() (map[string]any, error) {
			return converter.convert(link)
		}})
	}
	if len(items) == 0 {
		return nil, contentError("", "the content holds no share links")
	}
	return items, nil
}

// shareLinkSummary is what a link says about itself before conversion.
type shareLinkSummary struct {
	name   string
	server string
	port   int64
}

// shareLinkConverter reads the links of one scheme.
type shareLinkConverter struct {
	// protocol is the sing-box outbound type the links convert to.
	protocol string
	describe func(link string) (shareLinkSummary, error)
	convert  func(link string) (map[string]any, error)
}

// shareLinkConverters holds one converter per registered scheme, and
// shareLinkProtocols the protocols they convert to, in registration order.
var (
	shareLinkConverters = map[string]shareLinkConverter{}
	shareLinkProtocols  []string
)

// shareLinkProtocol reports whether format names a protocol share links convert to.
func shareLinkProtocol(format SourceFormat) bool {
	return slices.Contains(shareLinkProtocols, string(format))
}

func registerShareLink(converter shareLinkConverter, schemes ...string) {
	if !slices.Contains(shareLinkProtocols, converter.protocol) {
		shareLinkProtocols = append(shareLinkProtocols, converter.protocol)
	}
	for _, scheme := range schemes {
		if _, duplicate := shareLinkConverters[scheme]; duplicate {
			panic(fmt.Sprintf("share link scheme %q registered twice", scheme))
		}
		shareLinkConverters[scheme] = converter
	}
}

func init() {
	registerShareLink(shareLinkConverter{protocol: "shadowsocks", describe: describeShadowsocksLink, convert: convertShadowsocksLink}, "ss")
	registerShareLink(urlShareLink("socks", convertSocksLink), "socks", "socks5")
	registerShareLink(urlShareLink("http", convertHTTPLink), "http", "https")
	registerShareLink(shareLinkConverter{protocol: "vmess", describe: describeVMessLink, convert: convertVMessLink}, "vmess")
	registerShareLink(urlShareLink("vless", convertVLESSLink), "vless")
	registerShareLink(urlShareLink("trojan", convertTrojanLink), "trojan")
	registerShareLink(urlShareLink("hysteria", convertHysteriaLink), "hysteria")
	registerShareLink(urlShareLink("hysteria2", convertHysteria2Link), "hy2", "hysteria2")
	registerShareLink(urlShareLink("tuic", convertTUICLink), "tuic")
	registerShareLink(urlShareLink("anytls", convertAnyTLSLink), "anytls")
}

// urlShareLink is a converter for links of the form
// scheme://credentials@server:port?parameters#name.
func urlShareLink(protocol string, convert func(*url.URL) (map[string]any, error)) shareLinkConverter {
	return shareLinkConverter{
		protocol: protocol,
		describe: func(link string) (shareLinkSummary, error) {
			parsed, err := url.Parse(link)
			if err != nil {
				return shareLinkSummary{}, errors.New("malformed share link")
			}
			server, port, err := shareCoordinate(parsed)
			if err != nil {
				return shareLinkSummary{}, err
			}
			return shareLinkSummary{name: linkName(parsed), server: server, port: port}, nil
		},
		convert: func(link string) (map[string]any, error) {
			parsed, err := url.Parse(link)
			if err != nil {
				return nil, errors.New("malformed share link")
			}
			return convert(parsed)
		},
	}
}

// shareEndpoint starts an outbound of typeID at the link's server and port.
func shareEndpoint(parsed *url.URL, typeID string) (map[string]any, error) {
	server, port, err := shareCoordinate(parsed)
	if err != nil {
		return nil, err
	}
	return map[string]any{"type": typeID, "server": server, "server_port": port}, nil
}

func convertSocksLink(parsed *url.URL) (map[string]any, error) {
	result, err := shareEndpoint(parsed, "socks")
	if err != nil {
		return nil, err
	}
	copyUserInfo(result, parsed)
	return result, nil
}

func convertHTTPLink(parsed *url.URL) (map[string]any, error) {
	result, err := shareEndpoint(parsed, "http")
	if err != nil {
		return nil, err
	}
	copyUserInfo(result, parsed)
	return result, applyShareTLS(result, parsed.Query(), strings.EqualFold(parsed.Scheme, "https"))
}

func copyUserInfo(result map[string]any, parsed *url.URL) {
	username, password := shareUserInfo(parsed)
	if username != "" {
		result["username"] = username
	}
	if password != "" {
		result["password"] = password
	}
}

func convertVLESSLink(parsed *url.URL) (map[string]any, error) {
	result, err := shareEndpoint(parsed, "vless")
	if err != nil {
		return nil, err
	}
	username, _ := shareUserInfo(parsed)
	if username == "" {
		return nil, errors.New("the vless UUID is missing")
	}
	result["uuid"] = username
	query := parsed.Query()
	copyQuery(result, query, "flow", "flow")
	if err := applyShareTransport(result, query); err != nil {
		return nil, err
	}
	return result, applyShareTLS(result, query, false)
}

func convertTrojanLink(parsed *url.URL) (map[string]any, error) {
	result, err := passwordLink(parsed, "trojan")
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	if err := applyShareTransport(result, query); err != nil {
		return nil, err
	}
	return result, applyShareTLS(result, query, true)
}

func convertHysteriaLink(parsed *url.URL) (map[string]any, error) {
	result, err := shareEndpoint(parsed, "hysteria")
	if err != nil {
		return nil, err
	}
	auth, _ := shareUserInfo(parsed)
	if auth == "" {
		auth = parsed.Query().Get("auth")
	}
	if auth == "" {
		return nil, errors.New("the hysteria authentication is missing")
	}
	result["auth_str"] = auth
	return result, applyShareTLS(result, parsed.Query(), true)
}

func convertHysteria2Link(parsed *url.URL) (map[string]any, error) {
	result, err := passwordLink(parsed, "hysteria2")
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	if obfs := query.Get("obfs"); obfs != "" {
		if obfs != "salamander" || query.Get("obfs-password") == "" {
			return nil, errors.New("unsupported hysteria2 obfuscation")
		}
		result["obfs"] = map[string]any{"type": "salamander", "password": query.Get("obfs-password")}
	}
	return result, applyShareTLS(result, query, true)
}

func convertTUICLink(parsed *url.URL) (map[string]any, error) {
	result, err := shareEndpoint(parsed, "tuic")
	if err != nil {
		return nil, err
	}
	username, password := shareUserInfo(parsed)
	if username == "" || password == "" {
		return nil, errors.New("the tuic credentials are missing")
	}
	result["uuid"], result["password"] = username, password
	query := parsed.Query()
	copyQuery(result, query, "congestion_control", "congestion_control")
	copyQuery(result, query, "udp_relay_mode", "udp_relay_mode")
	return result, applyShareTLS(result, query, true)
}

func convertAnyTLSLink(parsed *url.URL) (map[string]any, error) {
	result, err := passwordLink(parsed, "anytls")
	if err != nil {
		return nil, err
	}
	return result, applyShareTLS(result, parsed.Query(), true)
}

// passwordLink starts an outbound whose password is the link's user.
func passwordLink(parsed *url.URL, typeID string) (map[string]any, error) {
	result, err := shareEndpoint(parsed, typeID)
	if err != nil {
		return nil, err
	}
	password, _ := shareUserInfo(parsed)
	if password == "" {
		return nil, errors.New("the password is missing")
	}
	result["password"] = password
	return result, nil
}

// shadowsocksLink reads both SIP002 links, ss://userinfo@server:port, and
// links that encode everything before the name, ss://base64#name.
func shadowsocksLink(link string) (*url.URL, string, string, error) {
	parsed, err := url.Parse(link)
	if err != nil {
		return nil, "", "", errors.New("malformed share link")
	}
	if parsed.User == nil && parsed.Port() == "" {
		decoded, err := decodeAnyBase64(parsed.Host + parsed.Path)
		if err != nil {
			return nil, "", "", errors.New("malformed shadowsocks link")
		}
		inner, err := url.Parse("ss://" + string(decoded))
		if err != nil {
			return nil, "", "", errors.New("malformed shadowsocks link")
		}
		inner.Fragment, inner.RawFragment, inner.RawQuery = parsed.Fragment, parsed.RawFragment, parsed.RawQuery
		parsed = inner
	}
	method, password := shareUserInfo(parsed)
	if password == "" && method != "" {
		if decoded, err := decodeAnyBase64(method); err == nil {
			if parts := strings.SplitN(string(decoded), ":", 2); len(parts) == 2 {
				method, password = parts[0], parts[1]
			}
		}
	}
	return parsed, method, password, nil
}

func describeShadowsocksLink(link string) (shareLinkSummary, error) {
	parsed, _, _, err := shadowsocksLink(link)
	if err != nil {
		return shareLinkSummary{}, err
	}
	server, port, err := shareCoordinate(parsed)
	if err != nil {
		return shareLinkSummary{}, err
	}
	return shareLinkSummary{name: linkName(parsed), server: server, port: port}, nil
}

func convertShadowsocksLink(link string) (map[string]any, error) {
	parsed, method, password, err := shadowsocksLink(link)
	if err != nil {
		return nil, err
	}
	if method == "" || password == "" {
		return nil, errors.New("the shadowsocks method or password is missing")
	}
	if plugin := parsed.Query().Get("plugin"); plugin != "" {
		return nil, errors.New("shadowsocks plugins are not supported")
	}
	result, err := shareEndpoint(parsed, "shadowsocks")
	if err != nil {
		return nil, err
	}
	result["method"], result["password"] = method, password
	return result, nil
}

// vmessLink decodes a vmess:// link's JSON payload.
func vmessLink(link string) (map[string]any, error) {
	payload := link[len("vmess://"):]
	if index := strings.IndexByte(payload, '#'); index >= 0 {
		payload = payload[:index]
	}
	decoded, err := decodeAnyBase64(payload)
	if err != nil || len(decoded) > MaximumDocumentBytes {
		return nil, errors.New("the vmess payload is not Base64 JSON")
	}
	object, err := DecodeDocumentObject(decoded)
	if err != nil {
		return nil, errors.New("the vmess payload is not Base64 JSON")
	}
	return object, nil
}

func describeVMessLink(link string) (shareLinkSummary, error) {
	object, err := vmessLink(link)
	if err != nil {
		return shareLinkSummary{}, err
	}
	server, _ := object["add"].(string)
	port, ok := DocumentInteger(object["port"], 1, 65535)
	if server == "" || !ok {
		return shareLinkSummary{}, errors.New("the vmess server or port is invalid")
	}
	name, _ := object["ps"].(string)
	return shareLinkSummary{name: strings.TrimSpace(name), server: server, port: port}, nil
}

func convertVMessLink(link string) (map[string]any, error) {
	object, err := vmessLink(link)
	if err != nil {
		return nil, err
	}
	server, _ := object["add"].(string)
	uuid, _ := object["id"].(string)
	port, ok := DocumentInteger(object["port"], 1, 65535)
	if server == "" || uuid == "" || !ok {
		return nil, errors.New("a vmess field is missing")
	}
	result := map[string]any{"type": "vmess", "server": server, "server_port": port, "uuid": uuid}
	if security, _ := object["scy"].(string); security != "" {
		result["security"] = security
	}
	if alterID, ok := DocumentInteger(object["aid"], 0, 65535); ok {
		result["alter_id"] = alterID
	}
	query := url.Values{}
	for _, pair := range [][2]string{{"type", "net"}, {"path", "path"}, {"host", "host"}, {"serviceName", "path"}, {"sni", "sni"}, {"alpn", "alpn"}, {"fp", "fp"}} {
		if value, _ := object[pair[1]].(string); value != "" {
			query.Set(pair[0], value)
		}
	}
	if network := query.Get("type"); network != "grpc" {
		query.Del("serviceName")
	} else {
		query.Del("path")
	}
	if err := applyShareTransport(result, query); err != nil {
		return nil, err
	}
	if mode, _ := object["tls"].(string); mode != "" && mode != "none" {
		query.Set("security", "tls")
	}
	return result, applyShareTLS(result, query, false)
}

func shareCoordinate(parsed *url.URL) (string, int64, error) {
	host := parsed.Hostname()
	port, err := strconv.ParseInt(parsed.Port(), 10, 64)
	if host == "" || err != nil || port < 1 || port > 65535 {
		return "", 0, errors.New("the server or port is invalid")
	}
	return host, port, nil
}

func shareUserInfo(parsed *url.URL) (string, string) {
	if parsed.User == nil {
		return "", ""
	}
	username := parsed.User.Username()
	password, _ := parsed.User.Password()
	return username, password
}

// linkName is the name in a link's fragment.
func linkName(parsed *url.URL) string {
	name, err := url.PathUnescape(parsed.EscapedFragment())
	if err != nil {
		name = parsed.Fragment
	}
	return strings.TrimSpace(name)
}

// applyShareTLS adds TLS when required or when the link's security asks for
// it, with Reality for security=reality.
func applyShareTLS(result map[string]any, query url.Values, required bool) error {
	security := strings.ToLower(query.Get("security"))
	switch security {
	case "", "none":
		if !required {
			return nil
		}
	case "tls", "reality", "xtls":
	default:
		return errors.New("unsupported link security")
	}
	tls := map[string]any{"enabled": true}
	if serverName := firstQuery(query, "sni", "peer", "serverName"); serverName != "" {
		tls["server_name"] = serverName
	}
	if insecure := firstQuery(query, "insecure", "allowInsecure"); insecure == "1" || strings.EqualFold(insecure, "true") {
		tls["insecure"] = true
	}
	if alpn := query.Get("alpn"); alpn != "" {
		values := []any{}
		for _, value := range strings.Split(alpn, ",") {
			if value = strings.TrimSpace(value); value != "" {
				values = append(values, value)
			}
		}
		tls["alpn"] = values
	}
	if fingerprint := query.Get("fp"); fingerprint != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": fingerprint}
	}
	if security == "reality" {
		reality := map[string]any{"enabled": true, "public_key": query.Get("pbk")}
		if shortID := query.Get("sid"); shortID != "" {
			reality["short_id"] = shortID
		}
		tls["reality"] = reality
	}
	result["tls"] = tls
	return nil
}

// applyShareTransport adds the link's WebSocket, gRPC or HTTPUpgrade
// transport.
func applyShareTransport(result map[string]any, query url.Values) error {
	network := strings.ToLower(query.Get("type"))
	switch network {
	case "", "tcp", "raw":
		return nil
	case "ws", "httpupgrade":
		transport := map[string]any{"type": network}
		if path := query.Get("path"); path != "" {
			transport["path"] = path
		}
		if host := query.Get("host"); host != "" {
			if network == "ws" {
				transport["headers"] = map[string]any{"Host": host}
			} else {
				transport["host"] = host
			}
		}
		result["transport"] = transport
	case "grpc":
		transport := map[string]any{"type": "grpc"}
		if service := query.Get("serviceName"); service != "" {
			transport["service_name"] = service
		}
		result["transport"] = transport
	default:
		return errors.New("unsupported link transport")
	}
	return nil
}

func copyQuery(target map[string]any, query url.Values, targetName, sourceName string) {
	if value := query.Get(sourceName); value != "" {
		target[targetName] = value
	}
}

func firstQuery(query url.Values, names ...string) string {
	for _, name := range names {
		if value := query.Get(name); value != "" {
			return value
		}
	}
	return ""
}

func decodeAnyBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	for _, encoding := range []*base64.Encoding{
		base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding,
	} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("invalid base64")
}

// SPDX-License-Identifier: GPL-3.0-or-later

package source

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/rehuony/sing-box-panel/internal/subscription/document"
	"github.com/rehuony/sing-box-panel/internal/subscription/node"
)

func parseURIListSource(raw []byte) ([]map[string]any, error) {
	decoded := bytes.TrimSpace(raw)
	if !bytes.Contains(decoded, []byte("://")) {
		if candidate, err := decodeAnyBase64(string(decoded)); err == nil && bytes.Contains(candidate, []byte("://")) {
			decoded = bytes.TrimSpace(candidate)
		}
	}
	lines := strings.Split(strings.ReplaceAll(string(decoded), "\r\n", "\n"), "\n")
	values := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		value, err := parseShareURI(line, len(values))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, errors.New("URI list is empty")
	}
	return values, nil
}

func parseShareURI(line string, index int) (map[string]any, error) {
	if strings.HasPrefix(strings.ToLower(line), "vmess://") {
		return parseVMessURI(line, index)
	}
	parsed, err := url.Parse(line)
	if err != nil || parsed.Scheme == "" {
		return nil, errors.New("malformed share URI")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "ss" {
		return parseShadowsocksURI(parsed, index)
	}
	typeMap := map[string]string{
		"socks": "socks", "socks5": "socks", "http": "http", "https": "http",
		"vless": "vless", "trojan": "trojan", "hysteria": "hysteria",
		"hy2": "hysteria2", "hysteria2": "hysteria2", "tuic": "tuic", "anytls": "anytls",
	}
	typeID, supported := typeMap[scheme]
	if !supported {
		return nil, errors.New("unsupported share URI scheme")
	}
	server, port, err := shareCoordinate(parsed)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"type": typeID, "tag": shareTag(parsed, typeID, index), "server": server, "server_port": port,
	}
	username, password := shareUserInfo(parsed)
	switch typeID {
	case "socks", "http":
		if username != "" {
			result["username"] = username
		}
		if password != "" {
			result["password"] = password
		}
	case "vless":
		if username == "" {
			return nil, errors.New("vless UUID is missing")
		}
		result["uuid"] = username
		copyQuery(result, parsed.Query(), "flow", "flow")
	case "trojan", "hysteria2", "anytls":
		if username == "" {
			return nil, errors.New("share URI password is missing")
		}
		result["password"] = username
	case "hysteria":
		if username == "" {
			return nil, errors.New("hysteria auth is missing")
		}
		result["auth_str"] = username
	case "tuic":
		if username == "" || password == "" {
			return nil, errors.New("tuic credentials are missing")
		}
		result["uuid"], result["password"] = username, password
	}
	applyShareTLS(result, parsed, scheme == "https" || typeID == "trojan" || typeID == "hysteria" || typeID == "hysteria2" || typeID == "tuic" || typeID == "anytls")
	return result, nil
}

func parseShadowsocksURI(parsed *url.URL, index int) (map[string]any, error) {
	server, port, err := shareCoordinate(parsed)
	if err != nil {
		return nil, err
	}
	username, password := shareUserInfo(parsed)
	if password == "" && username != "" {
		decoded, decodeErr := decodeAnyBase64(username)
		if decodeErr == nil {
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				username, password = parts[0], parts[1]
			}
		}
	}
	if username == "" || password == "" {
		return nil, errors.New("shadowsocks credentials are missing")
	}
	return map[string]any{
		"type": "shadowsocks", "tag": shareTag(parsed, "shadowsocks", index),
		"server": server, "server_port": port, "method": username, "password": password,
	}, nil
}

func parseVMessURI(line string, index int) (map[string]any, error) {
	decoded, err := decodeAnyBase64(strings.TrimPrefix(line, "vmess://"))
	if err != nil || len(decoded) > document.MaximumBytes {
		return nil, errors.New("invalid vmess payload")
	}
	value, err := document.Decode(decoded)
	if err != nil {
		return nil, errors.New("invalid vmess payload")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("invalid vmess payload")
	}
	server, _ := object["add"].(string)
	uuid, _ := object["id"].(string)
	port, ok := sourceInteger(object["port"], 1, 65535)
	if server == "" || uuid == "" || !ok {
		return nil, errors.New("vmess required field is missing")
	}
	tag, _ := object["ps"].(string)
	if !node.ValidTag(tag) {
		tag = fmt.Sprintf("vmess-%d", index+1)
	}
	result := map[string]any{
		"type": "vmess", "tag": tag, "server": server, "server_port": port, "uuid": uuid,
	}
	copyRenamed(result, object, "security", "scy")
	copyRenamed(result, object, "alter_id", "aid")
	if tlsMode, _ := object["tls"].(string); tlsMode != "" && tlsMode != "none" {
		tls := map[string]any{"enabled": true}
		if sni, _ := object["sni"].(string); sni != "" {
			tls["server_name"] = sni
		}
		result["tls"] = tls
	}
	return result, nil
}

func shareCoordinate(parsed *url.URL) (string, int64, error) {
	host := parsed.Hostname()
	port, err := strconv.ParseInt(parsed.Port(), 10, 64)
	if host == "" || err != nil || port < 1 || port > 65535 {
		return "", 0, errors.New("share URI coordinate is invalid")
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

func shareTag(parsed *url.URL, typeID string, index int) string {
	if tag, err := url.PathUnescape(parsed.Fragment); err == nil && node.ValidTag(tag) {
		return tag
	}
	return fmt.Sprintf("%s-%d", typeID, index+1)
}

func applyShareTLS(result map[string]any, parsed *url.URL, required bool) {
	query := parsed.Query()
	security := strings.ToLower(query.Get("security"))
	if !required && security != "tls" && security != "reality" {
		return
	}
	tls := map[string]any{"enabled": true}
	if serverName := firstQuery(query, "sni", "peer", "serverName"); serverName != "" {
		tls["server_name"] = serverName
	}
	if insecure := firstQuery(query, "insecure", "allowInsecure"); insecure == "1" || strings.EqualFold(insecure, "true") {
		tls["insecure"] = true
	}
	result["tls"] = tls
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

// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

func sanitizeLogMessage(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("log message is empty")
	}
	if !utf8.ValidString(value) {
		return "", errors.New("log message is not valid UTF-8")
	}
	for _, character := range value {
		if character == '\n' || character == '\r' || character == 0 || (character < 0x20 && character != '\t') {
			return "", errors.New("log message contains a forbidden control character")
		}
	}
	if looksLikeConfigOrSubscriptionText(value) {
		return "", errors.New("log message must not contain a full configuration or subscription body")
	}
	value = sanitizeEmbeddedLogSecrets(value)
	if len(value) > MaximumLogMessageBytes {
		return "", fmt.Errorf("log message exceeds %d bytes", MaximumLogMessageBytes)
	}
	return value, nil
}

func redactLogAssignment(value string) string {
	separator := strings.IndexAny(value, ":=")
	if separator == -1 {
		return redactedLogValue
	}
	return strings.TrimSpace(value[:separator]) + value[separator:separator+1] + redactedLogValue
}

func sanitizeLogMetadata(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) > 0 && !utf8.Valid(raw) {
		return nil, errors.New("log metadata is not valid UTF-8")
	}
	canonical, err := canonicalJSONObjectWithLimit(raw, `{}`, MaximumLogMetadataBytes)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(canonical, &object); err != nil {
		return nil, err
	}
	nodes := 0
	sanitized, err := sanitizeLogValue(object, 0, &nodes)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		return nil, err
	}
	if len(encoded) > MaximumLogMetadataBytes {
		return nil, fmt.Errorf("sanitized log metadata exceeds %d bytes", MaximumLogMetadataBytes)
	}
	return append(json.RawMessage(nil), encoded...), nil
}

func sanitizeLogValue(value any, depth int, nodes *int) (any, error) {
	if depth > maximumLogMetadataDepth {
		return nil, errors.New("log metadata nesting is too deep")
	}
	*nodes++
	if *nodes > maximumLogMetadataNodes {
		return nil, errors.New("log metadata contains too many values")
	}
	switch typed := value.(type) {
	case map[string]any:
		if looksLikeFullLogBody(typed) {
			return nil, errors.New("log metadata contains a full configuration or subscription body")
		}
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if !utf8.ValidString(key) {
				return nil, errors.New("log metadata key is not valid UTF-8")
			}
			normalized := normalizeLogMetadataKey(key)
			switch {
			case sensitiveLogMetadataKey(normalized):
				result[key] = redactedLogValue
			case prohibitedLogBodyKey(normalized):
				result[key] = omittedLogValue
			default:
				clean, err := sanitizeLogValue(child, depth+1, nodes)
				if err != nil {
					return nil, err
				}
				result[key] = clean
			}
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			clean, err := sanitizeLogValue(child, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			result[index] = clean
		}
		return result, nil
	case string:
		if !utf8.ValidString(typed) {
			return nil, errors.New("log metadata string is not valid UTF-8")
		}
		if looksLikeConfigOrSubscriptionText(typed) {
			return omittedLogValue, nil
		}
		return sanitizeEmbeddedLogSecrets(typed), nil
	default:
		return typed, nil
	}
}

func normalizeLogMetadataKey(value string) string {
	var result strings.Builder
	result.Grow(len(value))
	for _, character := range strings.ToLower(value) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			result.WriteRune(character)
		}
	}
	return result.String()
}

func sensitiveLogMetadataKey(value string) bool {
	if value == "" {
		return false
	}
	for _, exact := range []string{
		"token", "accesstoken", "refreshtoken", "idtoken", "apikey", "accesskey",
		"password", "passwd", "secret", "clientsecret", "privatekey", "authorization",
		"proxyauthorization", "proxycredentials", "proxycredential", "cookie", "setcookie",
	} {
		if value == exact {
			return true
		}
	}
	for _, fragment := range []string{"token", "password", "passwd", "secret", "privatekey", "authorization", "cookie", "credential"} {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return strings.Contains(value, "proxy") && (strings.Contains(value, "credential") || strings.Contains(value, "authorization") || strings.Contains(value, "password") || strings.Contains(value, "username"))
}

func prohibitedLogBodyKey(value string) bool {
	for _, key := range []string{
		"config", "configuration", "configbytes", "configjson", "rawconfig", "renderedconfig",
		"document", "documentjson", "canonicaldocument", "subscription", "subscriptionbody",
		"subscriptioncontent", "rawsubscription", "content", "contentjson", "body", "rawbody",
		"rawbytes", "renderedbody", "responsebody", "requestbody",
	} {
		if value == key {
			return true
		}
	}
	return false
}

func sanitizeEmbeddedLogSecrets(value string) string {
	value = logHeaderPattern.ReplaceAllStringFunc(value, redactLogHeader)
	value = logBearerPattern.ReplaceAllString(value, "Bearer "+redactedLogValue)
	value = logAssignmentPattern.ReplaceAllStringFunc(value, redactLogAssignment)
	return logURLUserInfoPattern.ReplaceAllString(value, "${1}://"+redactedLogValue+"@")
}

func redactLogHeader(value string) string {
	separator := strings.Index(value, ":")
	if separator == -1 {
		return redactedLogValue
	}
	return strings.TrimSpace(value[:separator]) + ":" + redactedLogValue
}

func looksLikeFullLogBody(object map[string]any) bool {
	for _, key := range []string{"outbounds", "inbounds", "endpoints", "proxies", "proxy-groups", "proxy-providers"} {
		if value, present := object[key]; present {
			if _, isArray := value.([]any); isArray {
				return true
			}
		}
	}
	_, hasSchemaVersion := object["schema_version"]
	_, hasNodes := object["nodes"].([]any)
	_, hasRules := object["rules"].([]any)
	return hasSchemaVersion && (hasNodes || hasRules)
}

func looksLikeConfigOrSubscriptionText(value string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return false
	}
	for _, prefix := range []string{
		"ss://", "ssr://", "vmess://", "vless://", "trojan://", "hysteria://",
		"hysteria2://", "tuic://", "wireguard://",
	} {
		if strings.HasPrefix(trimmed, prefix) || strings.Contains(trimmed, "\n"+prefix) {
			return true
		}
	}
	if strings.Contains(trimmed, "\"outbounds\"") && (strings.Contains(trimmed, "\"type\"") || strings.Contains(trimmed, "\"server\"")) {
		return true
	}
	return strings.HasPrefix(trimmed, "proxies:") || strings.Contains(trimmed, "\nproxies:") || strings.Contains(trimmed, "\nproxy-groups:")
}

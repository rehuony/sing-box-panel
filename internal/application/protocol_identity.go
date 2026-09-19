// SPDX-License-Identifier: GPL-3.0-or-later

package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/rehuony/sing-box-panel/internal/configuration"
	"github.com/rehuony/sing-box-panel/internal/store"
)

var ErrIdentityConfiguration = errors.New("protocol identity requires a valid saved configuration")

// NewInboundDefaults prepares one editor entry without saving or starting it.
func (app *Application) NewInboundDefaults(ctx context.Context, typeID string) (map[string]any, error) {
	if typeID == "" || len(typeID) > 64 {
		return nil, ErrPanelSettingsInvalid
	}
	inbound := map[string]any{"type": typeID}
	value, _, err := app.storedPanelSettings(ctx)
	if err != nil {
		return nil, err
	}
	if value.IdentityKey == "" {
		return inbound, nil
	}
	if typeID == "shadowsocks" {
		inbound["method"] = "2022-blake3-aes-128-gcm"
	}
	input, err := json.Marshal(map[string]any{"inbounds": []any{inbound}})
	if err != nil {
		return nil, err
	}
	content, _, err := applyProtocolIdentity(string(input), value.Preferences.IdentityName, value.IdentityKey)
	if err != nil {
		return nil, err
	}
	document, err := configuration.Parse([]byte(content))
	if err != nil {
		return nil, err
	}
	return document.Configuration()["inbounds"].([]any)[0].(map[string]any), nil
}

func (app *Application) identityConfigurationUpdate(ctx context.Context, value storedPanelSettings) (*store.ConfigurationFileUpdate, error) {
	if value.IdentityKey == "" {
		return nil, nil
	}
	file, err := app.database.ConfigurationFile(ctx)
	if err != nil {
		return nil, err
	}
	content, changed, err := applyProtocolIdentity(file.Content, value.Preferences.IdentityName, value.IdentityKey)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, nil
	}
	return app.configurationFileUpdate(ConfigurationFileWrite{Revision: file.Revision, Content: content})
}

// Explicit identity saves update a named default user in the saved file. Other
// users and imported outbounds are preserved. A renamed identity is a new user;
// the previous name remains available to existing subscribers until removed in
// the native editor. Runtime/publication still uses the last loaded artifact.
func applyProtocolIdentity(content, name, key string) (string, bool, error) {
	document, err := configuration.Parse([]byte(content))
	if err != nil {
		return "", false, ErrIdentityConfiguration
	}
	root := document.Configuration()
	name = strings.TrimSpace(name)
	if name == "" {
		name = "panel"
	}
	identifier, err := uuid.Parse(key)
	if err != nil {
		identifier = uuid.NewSHA1(uuid.NameSpaceURL, []byte("sing-box-panel:protocol-identity:"+key))
	}
	changed := false
	inbounds, _ := root["inbounds"].([]any)
	for _, raw := range inbounds {
		inbound, ok := raw.(map[string]any)
		if !ok {
			return "", false, ErrIdentityConfiguration
		}
		typ, _ := inbound["type"].(string)
		credentials := map[string]any{"name": name}
		field := "name"
		users, validUsers := inbound["users"].([]any)
		if inbound["users"] != nil && !validUsers {
			return "", false, ErrIdentityConfiguration
		}
		switch typ {
		case "mixed", "socks", "http", "naive":
			// Do not turn an intentionally unauthenticated listener into an
			// authenticated one merely by saving unrelated identity defaults.
			if typ != "naive" && len(users) == 0 {
				continue
			}
			field = "username"
			delete(credentials, "name")
			credentials["username"], credentials["password"] = name, key
		case "vmess", "vless":
			credentials["uuid"] = identifier.String()
		case "tuic":
			credentials["uuid"], credentials["password"] = identifier.String(), key
		case "trojan", "hysteria2", "anytls":
			credentials["password"] = key
		case "shadowtls":
			if inbound["version"] != json.Number("3") {
				if inbound["password"] != key {
					inbound["password"] = key
					changed = true
				}
				continue
			}
			credentials["password"] = key
		case "hysteria":
			credentials["auth_str"] = key
		case "snell":
			if len(users) == 0 {
				if inbound["psk"] != key {
					inbound["psk"] = key
					changed = true
				}
				continue
			}
			credentials["userkey"] = key
		case "shadowsocks":
			if inbound["managed"] == true || inbound["destinations"] != nil {
				continue
			}
			password := key
			method, _ := inbound["method"].(string)
			if strings.HasPrefix(method, "2022-") {
				size := 32
				if method == "2022-blake3-aes-128-gcm" {
					size = 16
				}
				mac := hmac.New(sha256.New, []byte(key))
				_, _ = mac.Write([]byte("sing-box-panel:" + method))
				password = base64.StdEncoding.EncodeToString(mac.Sum(nil)[:size])
			}
			if len(users) == 0 {
				if inbound["password"] != password {
					inbound["password"] = password
					changed = true
				}
				continue
			}
			credentials["password"] = password
		default:
			continue
		}
		var target map[string]any
		for _, rawUser := range users {
			user, ok := rawUser.(map[string]any)
			if !ok {
				return "", false, ErrIdentityConfiguration
			}
			if user[field] == name {
				target = user
				break
			}
		}
		if target == nil {
			// The existing unnamed password user had the stable "default" identity.
			// Retain that identity when adding a second named user.
			if len(users) == 1 && field == "name" {
				old := users[0].(map[string]any)
				if old["name"] == nil && old["uuid"] == nil {
					old["name"] = "default"
				}
			}
			target = make(map[string]any)
			users = append(users, target)
			inbound["users"] = users
			changed = true
		}
		for field, value := range credentials {
			if target[field] != value {
				target[field] = value
				changed = true
			}
		}
		if typ == "hysteria" && target["auth"] != nil {
			delete(target, "auth")
			changed = true
		}
	}
	if !changed {
		return content, false, nil
	}
	encoded, err := json.MarshalIndent(root, "", "  ")
	return string(encoded) + "\n", true, err
}

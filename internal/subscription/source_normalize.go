// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

func normalizedSourceNodes(values []map[string]any, sourceID string) ([]Node, error) {
	if len(values) > MaximumNodes {
		return nil, ErrInvalidSource
	}
	nodes := make([]Node, 0, len(values))
	seenIdentity := make(map[string]struct{}, len(values))
	seenTags := make(map[string]struct{}, len(values))
	for _, value := range values {
		typeID, _ := value["type"].(string)
		tag, _ := value["tag"].(string)
		server, _ := value["server"].(string)
		_, portOK := sourceInteger(value["server_port"], 1, 65535)
		if !ValidType(typeID) || !ValidTag(tag) || server == "" || !portOK {
			return nil, ErrInvalidSource
		}
		if err := validateSourceRequiredFields(typeID, value); err != nil {
			return nil, ErrInvalidSource
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, ErrInvalidSource
		}
		digest := sha256.Sum256(encoded)
		identity := hex.EncodeToString(digest[:])
		if _, duplicate := seenIdentity[identity]; duplicate {
			return nil, fmt.Errorf("%w: duplicate node identity", ErrInvalidSource)
		}
		if _, duplicate := seenTags[tag]; duplicate {
			return nil, fmt.Errorf("%w: duplicate node tag", ErrInvalidSource)
		}
		seenIdentity[identity] = struct{}{}
		seenTags[tag] = struct{}{}
		nodes = append(nodes, Node{
			Key: "source:" + sourceID + ":" + identity[:24], SourceID: sourceID,
			Type: typeID, Tag: tag, Outbound: encoded,
		})
	}
	if _, err := PublicationDocument(nodes); err != nil {
		return nil, fmt.Errorf("%w: normalized nodes", ErrInvalidSource)
	}
	return nodes, nil
}

func validateSourceRequiredFields(typeID string, value map[string]any) error {
	if err := validateConvertedCredential(typeID, value); err != nil {
		return err
	}
	if typeID == "shadowsocks" {
		if method, ok := value["method"].(string); !ok || method == "" {
			return errors.New("shadowsocks method is missing")
		}
	}
	return nil
}

func validNodeSourceID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

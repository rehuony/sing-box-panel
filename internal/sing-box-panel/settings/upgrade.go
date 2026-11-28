// SPDX-License-Identifier: GPL-3.0-or-later

package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"

	"github.com/rehuony/sing-box-panel/internal/jsonstrict"
	"github.com/rehuony/sing-box-panel/internal/settingsfile"
)

// upgradeDocument returns a settings document of the previous release in the
// current shape: logs.panel_retention_days, which kept both the panel log and
// the runtime history, becomes nodes.runtime_history_retention_days unless the
// document already sets that. Any other document, including one that is not a
// valid JSON object, is returned unchanged for the strict decoder to judge.
func upgradeDocument(data []byte) []byte {
	var document map[string]json.RawMessage
	if jsonstrict.Decode(data, MaximumBytes, &document) != nil || document == nil {
		return data
	}
	var logs, nodes map[string]json.RawMessage
	if json.Unmarshal(document["logs"], &logs) != nil {
		return data
	}
	retention, found := logs["panel_retention_days"]
	if !found {
		return data
	}
	if raw, present := document["nodes"]; present {
		if json.Unmarshal(raw, &nodes) != nil || nodes == nil {
			return data
		}
	} else {
		nodes = map[string]json.RawMessage{}
	}
	delete(logs, "panel_retention_days")
	if _, set := nodes["runtime_history_retention_days"]; !set {
		nodes["runtime_history_retention_days"] = retention
	}
	var err error
	if document["logs"], err = json.Marshal(logs); err != nil {
		return data
	}
	if document["nodes"], err = json.Marshal(nodes); err != nil {
		return data
	}
	compact, err := json.Marshal(document)
	if err != nil {
		return data
	}
	upgraded, err := settingsfile.Format(reflect.TypeFor[Settings](), compact)
	if err != nil {
		return data
	}
	return upgraded
}

// UpgradeFile rewrites the settings file at path in the current shape when
// the previous release wrote it, and leaves a current file untouched. The
// caller first recovers an interrupted settings save.
func UpgradeFile(ctx context.Context, path string) error {
	lock, err := settingsfile.Lock(ctx, path)
	if err != nil {
		return err
	}
	defer lock.Close()
	before, err := Read(path)
	if err != nil {
		return err
	}
	after := upgradeDocument(before)
	if bytes.Equal(before, after) {
		return nil
	}
	return ReplaceLocked(path, after)
}

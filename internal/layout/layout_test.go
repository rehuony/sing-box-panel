// SPDX-License-Identifier: GPL-3.0-or-later

// Package layout enforces the dependency rules of the code layout in
// docs/panel-node-architecture.md. It holds no code of its own.
package layout

import (
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
)

const modulePath = "github.com/rehuony/sing-box-panel"

type owner int

const (
	shared owner = iota
	panel
	node
)

const (
	panelBinary = modulePath + "/cmd/sing-box-panel"
	nodeBinary  = modulePath + "/cmd/sing-box-node"
	panelTree   = modulePath + "/internal/sing-box-panel"
	nodeTree    = modulePath + "/internal/sing-box-node"
	// The panel still runs this host's sing-box in process through localnode,
	// the only panel package allowed to import the node until node sessions
	// carry commands.
	localNode = panelTree + "/localnode"
)

func within(path, tree string) bool {
	return path == tree || strings.HasPrefix(path, tree+"/")
}

func classify(path string) owner {
	switch {
	case path == panelBinary || within(path, panelTree):
		return panel
	case path == nodeBinary || within(path, nodeTree):
		return node
	default:
		return shared
	}
}

// Assets that only the panel serves. The node binary must not embed them.
var panelOnlyAssets = []string{
	modulePath + "/web",
}

type listedPackage struct {
	ImportPath string
	Imports    []string
	Deps       []string
}

func TestServiceDependencyRules(t *testing.T) {
	command := exec.Command("go", "list", "-json", "./...")
	command.Dir = "../.."
	var stderr bytes.Buffer
	command.Stderr = &stderr
	output, err := command.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.String())
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	checked := 0
	for {
		var pkg listedPackage
		if err := decoder.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		checked++
		// cmd holds only the two binaries' main packages.
		if within(pkg.ImportPath, modulePath+"/cmd") && pkg.ImportPath != panelBinary && pkg.ImportPath != nodeBinary {
			t.Errorf("package %s sits under cmd; service code belongs in internal/sing-box-panel or internal/sing-box-node", pkg.ImportPath)
		}
		// Tools are programs and may use any package.
		if within(pkg.ImportPath, modulePath+"/tools") {
			continue
		}
		from := classify(pkg.ImportPath)
		// The services import each other only through localnode.
		for _, dependency := range pkg.Imports {
			if !strings.HasPrefix(dependency, modulePath+"/") {
				continue
			}
			switch to := classify(dependency); {
			case from == panel && to == node && pkg.ImportPath != localNode:
				t.Errorf("panel package %s imports node package %s; only localnode may", pkg.ImportPath, dependency)
			case from == node && to == panel:
				t.Errorf("node package %s imports panel package %s", pkg.ImportPath, dependency)
			}
		}
		// Shared packages depend on neither service.
		if from == shared {
			for _, dependency := range pkg.Deps {
				if strings.HasPrefix(dependency, modulePath+"/") && classify(dependency) != shared {
					t.Errorf("shared package %s depends on service package %s", pkg.ImportPath, dependency)
				}
			}
		}
		if pkg.ImportPath == nodeBinary {
			for _, dependency := range pkg.Deps {
				if classify(dependency) == panel {
					t.Errorf("sing-box-node depends on panel package %s", dependency)
				}
				for _, asset := range panelOnlyAssets {
					if dependency == asset {
						t.Errorf("sing-box-node embeds panel asset package %s", asset)
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("go list reported no packages")
	}
}

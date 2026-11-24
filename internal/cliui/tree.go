// SPDX-License-Identifier: GPL-3.0-or-later

package cliui

import (
	"path/filepath"
	"slices"
	"strings"
)

// TreeEntry is one reported path of a file tree. Label holds its
// comma-separated labels; Sized marks Size as the path's known size.
type TreeEntry struct {
	Path      string
	Label     string
	Directory bool
	Size      int64
	Sized     bool
}

type treeNode struct {
	path      string
	labels    []string
	children  []*treeNode
	directory bool
	reported  bool
	size      int64
	sized     bool
}

// total is the node's own size, or the sum of the sized paths below it.
func (node *treeNode) total() (int64, bool) {
	if node.sized {
		return node.size, true
	}
	var sum int64
	known := false
	for _, child := range node.children {
		if size, ok := child.total(); ok {
			sum += size
			known = true
		}
	}
	return sum, known
}

// Tree groups only reported paths, sorted by name. Unlabeled ancestors are
// structural and a chain of them is shown as one line; rendering never
// inspects the filesystem or expands a symlink's target. A reported path
// shows its labels and its size: its own, or the total of the sized paths
// reported below it.
func (style Style) Tree(entries []TreeEntry) string {
	nodes := make(map[string]*treeNode)
	var roots []*treeNode
	var ensure func(string) *treeNode
	ensure = func(path string) *treeNode {
		if node, ok := nodes[path]; ok {
			return node
		}
		node := &treeNode{path: path}
		nodes[path] = node
		if parent := filepath.Dir(path); parent != path {
			parentNode := ensure(parent)
			parentNode.children = append(parentNode.children, node)
		} else {
			roots = append(roots, node)
		}
		return node
	}
	for _, entry := range entries {
		node := ensure(filepath.Clean(entry.Path))
		node.reported = true
		if entry.Label != "" {
			node.labels = append(node.labels, entry.Label)
		}
		node.directory = node.directory || entry.Directory
		if entry.Sized {
			node.size, node.sized = entry.Size, true
		}
	}
	for _, node := range nodes {
		slices.Sort(node.labels)
		node.labels = slices.Compact(node.labels)
		slices.SortFunc(node.children, func(a, b *treeNode) int { return strings.Compare(a.path, b.path) })
	}
	slices.SortFunc(roots, func(a, b *treeNode) int { return strings.Compare(a.path, b.path) })
	var text strings.Builder
	var write func(*treeNode, string, string, string)
	write = func(node *treeNode, name, prefix, branch string) {
		// Keep the filesystem root visible; compact only the branches below it.
		for branch != "" && !node.reported && len(node.children) == 1 {
			node = node.children[0]
			name = filepath.Join(name, filepath.Base(node.path))
		}
		if node.directory || !node.reported {
			if !strings.HasSuffix(name, string(filepath.Separator)) {
				name += string(filepath.Separator)
			}
			name = style.paint(codeDirectory, name)
		}
		text.WriteString(style.paint(codeMuted, prefix+branch) + name)
		labels := node.labels
		if size, ok := node.total(); ok && node.reported {
			labels = append(slices.Clip(labels), FormatBytes(size))
		}
		if len(labels) > 0 {
			text.WriteString(" " + style.Label(strings.Join(labels, ", ")))
		}
		text.WriteByte('\n')
		switch branch {
		case "├── ":
			prefix += "│   "
		case "└── ":
			prefix += "    "
		}
		for i, child := range node.children {
			childBranch := "├── "
			if i == len(node.children)-1 {
				childBranch = "└── "
			}
			write(child, filepath.Base(child.path), prefix, childBranch)
		}
	}
	for i, root := range roots {
		if i > 0 {
			text.WriteByte('\n')
		}
		write(root, root.path, "", "")
	}
	return strings.TrimSuffix(text.String(), "\n")
}

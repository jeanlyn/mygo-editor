package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const DefaultTreeEntryLimit = 1000

// treeNode is UI-thread-owned. Directory readers return detached nodes; they
// never mutate the live tree. Symlinks are visible but never expandable.
type treeNode struct {
	Path, Name                string
	IsDir, Symlink            bool
	Expanded, Loaded, Loading bool
	Err                       error
	Children                  []*treeNode
	Truncated                 bool
	owner                     *ProjectTree
}

type ProjectTree struct{ Root *treeNode }

type treeRow struct {
	Node  *treeNode
	Depth int
}

// NewProjectTree validates only the chosen directory, not its descendants.
// An explicitly chosen symlink root is resolved once to its actual directory.
func NewProjectTree(path string) (*ProjectTree, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("不是文件夹: %s", absolute)
	}
	t := &ProjectTree{}
	t.Root = &treeNode{Path: absolute, Name: filepath.Base(absolute), IsDir: true, Expanded: true, owner: t}
	return t, nil
}

// BeginLoad is called on the UI thread before launching a directory reader.
// Loaded directories are cached; errors may be retried on the next expansion.
func (t *ProjectTree) BeginLoad(n *treeNode) bool {
	if t == nil || n == nil || n.owner != t || !n.IsDir || n.Symlink || n.Loaded || n.Loading {
		return false
	}
	n.Loading, n.Err = true, nil
	return true
}

// ApplyLoad runs on the UI thread. Capture the tree as well as the node when
// launching work, and check currentTree == capturedTree before applying results
// or updating the UI after a project switch. Nodes from other trees are rejected.
func (t *ProjectTree) ApplyLoad(n *treeNode, children []*treeNode, truncated bool, err error) bool {
	if t == nil || n == nil || n.owner != t || !n.Loading {
		return false
	}
	n.Loading, n.Loaded, n.Err = false, err == nil, err
	n.Children, n.Truncated = children, truncated
	for _, child := range children {
		child.owner = t
	}
	return true
}

func (t *ProjectTree) SetExpanded(n *treeNode, expanded bool) bool {
	if t == nil || n == nil || n.owner != t || !n.IsDir || n.Symlink {
		return false
	}
	n.Expanded = expanded
	return true
}

// VisibleRows includes the root and only expanded descendants. It performs no
// I/O, and uses a stack so deeply nested expanded folders do not recurse.
func (t *ProjectTree) VisibleRows() []treeRow {
	if t == nil || t.Root == nil {
		return nil
	}
	stack := []treeRow{{Node: t.Root}}
	var rows []treeRow
	for len(stack) != 0 {
		row := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		rows = append(rows, row)
		if row.Node.Expanded {
			for i := len(row.Node.Children) - 1; i >= 0; i-- {
				stack = append(stack, treeRow{Node: row.Node.Children[i], Depth: row.Depth + 1})
			}
		}
	}
	return rows
}

// ReadTreeDirectory may run in a goroutine. It reads at most limit+1 directory
// entries and never scans descendants. A truncated result is an explicitly
// flagged, sorted subset, not a claim to contain the first names alphabetically.
// The UI must display a truncation notice when the returned bool is true.
func ReadTreeDirectory(path string, limit int) ([]*treeNode, bool, error) {
	if limit <= 0 || limit > DefaultTreeEntryLimit {
		limit = DefaultTreeEntryLimit
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, false, fmt.Errorf("不是可展开的文件夹: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	entries, err := f.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	truncated := len(entries) > limit
	if truncated {
		entries = entries[:limit]
	}
	nodes := make([]*treeNode, 0, len(entries))
	for _, entry := range entries {
		symlink := entry.Type()&os.ModeSymlink != 0
		nodes = append(nodes, &treeNode{Path: filepath.Join(path, entry.Name()), Name: entry.Name(), IsDir: entry.IsDir() && !symlink, Symlink: symlink})
	}
	sort.Slice(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		x, y := strings.ToLower(a.Name), strings.ToLower(b.Name)
		if x != y {
			return x < y
		}
		return a.Name < b.Name
	})
	return nodes, truncated, nil
}

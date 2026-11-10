package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func treeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestTreeSortingAndHiddenFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"zeta", "Alpha", ".hidden-dir"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"z.go", "A.go", "a.go", ".gitignore"} {
		treeFile(t, filepath.Join(dir, name))
	}
	nodes, truncated, err := ReadTreeDirectory(dir, 100)
	if err != nil || truncated {
		t.Fatalf("read: %v, truncated %v", err, truncated)
	}
	var names []string
	for _, n := range nodes {
		names = append(names, n.Name)
		if n.Path != filepath.Join(dir, n.Name) {
			t.Fatal("incorrect child path")
		}
		if n.Loaded || n.Loading || n.Expanded || len(n.Children) != 0 {
			t.Fatal("children must be lazy")
		}
	}
	want := []string{".hidden-dir", "Alpha", "zeta", ".gitignore", "A.go", "a.go", "z.go"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names %v, want %v", names, want)
	}
}

func TestTreeLazyExpandCollapseAndAsyncApply(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	treeFile(t, filepath.Join(dir, "nested", "child.go"))
	tree, err := NewProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Root.Loaded || len(tree.Root.Children) != 0 || len(tree.VisibleRows()) != 1 {
		t.Fatal("constructor eagerly loaded tree")
	}
	if !tree.BeginLoad(tree.Root) || tree.BeginLoad(tree.Root) {
		t.Fatal("duplicate load not prevented")
	}
	children, truncated, err := ReadTreeDirectory(dir, 100)
	if !tree.ApplyLoad(tree.Root, children, truncated, err) || tree.BeginLoad(tree.Root) {
		t.Fatal("loaded root should be cached")
	}
	nested := children[0]
	if nested.Loaded || len(nested.Children) != 0 {
		t.Fatal("recursive scan")
	}
	if !tree.SetExpanded(nested, true) || !tree.BeginLoad(nested) {
		t.Fatal("expand failed")
	}
	grandchildren, truncated, err := ReadTreeDirectory(nested.Path, 100)
	tree.SetExpanded(nested, false) // In-flight completion must not reopen it.
	if !tree.ApplyLoad(nested, grandchildren, truncated, err) {
		t.Fatal("apply failed")
	}
	if nested.Expanded || len(tree.VisibleRows()) != 2 {
		t.Fatal("load reopened collapsed folder")
	}
	tree.SetExpanded(nested, true)
	rows := tree.VisibleRows()
	if len(rows) != 3 || rows[2].Depth != 2 || rows[2].Node.Name != "child.go" {
		t.Fatalf("rows: %#v", rows)
	}
	tree.SetExpanded(tree.Root, false)
	if len(tree.VisibleRows()) != 1 {
		t.Fatal("root collapse failed")
	}
	if tree.SetExpanded(grandchildren[0], true) {
		t.Fatal("file expanded")
	}
}

func TestTreeLimitsAndEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	nodes, truncated, err := ReadTreeDirectory(dir, 2)
	if err != nil || truncated || len(nodes) != 0 {
		t.Fatal("empty directory failed")
	}
	for _, name := range []string{"a", "b"} {
		treeFile(t, filepath.Join(dir, name))
	}
	nodes, truncated, err = ReadTreeDirectory(dir, 2)
	if err != nil || truncated || len(nodes) != 2 {
		t.Fatal("exact limit marked truncated")
	}
	treeFile(t, filepath.Join(dir, "c"))
	nodes, truncated, err = ReadTreeDirectory(dir, 2)
	if err != nil || !truncated || len(nodes) != 2 {
		t.Fatal("bounded read missing truncation flag")
	}
}

func TestTreeRejectsStaleNodesAndRetriesErrors(t *testing.T) {
	dir := t.TempDir()
	a, _ := NewProjectTree(dir)
	b, _ := NewProjectTree(dir)
	a.BeginLoad(a.Root)
	if b.BeginLoad(a.Root) || b.SetExpanded(a.Root, false) || b.ApplyLoad(a.Root, nil, false, nil) {
		t.Fatal("new project accepted old node")
	}
	failed := errors.New("directory unavailable")
	if !a.ApplyLoad(a.Root, nil, false, failed) || a.Root.Loading || a.Root.Loaded || a.Root.Err != failed {
		t.Fatal("error state not retained")
	}
	if !a.BeginLoad(a.Root) {
		t.Fatal("error cannot retry")
	}
	if !a.ApplyLoad(a.Root, nil, false, nil) || !a.Root.Loaded || a.Root.Err != nil {
		t.Fatal("retry failed")
	}
	if a.ApplyLoad(a.Root, nil, false, failed) {
		t.Fatal("duplicate completion accepted")
	}
}

func TestTreeSymlinksAreNotTraversed(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(dir, filepath.Join(dir, "loop")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	nodes, _, err := ReadTreeDirectory(dir, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || !nodes[0].Symlink || nodes[0].IsDir {
		t.Fatal("symlink treated as directory")
	}
	if _, _, err := ReadTreeDirectory(nodes[0].Path, 100); err == nil {
		t.Fatal("reader followed symlink")
	}
	// Choosing a symlink root explicitly is supported and canonicalized.
	tree, err := NewProjectTree(nodes[0].Path)
	if err != nil || tree.Root.Path != dir {
		t.Fatalf("explicit root symlink: %v", err)
	}
}

func TestTreeInvalidRootAndRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	treeFile(t, path)
	if _, err := NewProjectTree(path); err == nil {
		t.Fatal("accepted file root")
	}
	if _, _, err := ReadTreeDirectory(path, 100); err == nil {
		t.Fatal("listed file")
	}
	if _, err := NewProjectTree(path + "-missing"); err == nil {
		t.Fatal("accepted missing root")
	}
	if _, _, err := ReadTreeDirectory(path+"-missing", 100); err == nil {
		t.Fatal("listed missing root")
	}
}

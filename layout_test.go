package main

import (
	"github.com/egoist/mygo/ui"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceLayoutAndTreeAlignment(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.go", "a_very_long_filename.go"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("package sample\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	tree, err := NewProjectTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	tree.BeginLoad(tree.Root)
	children, cut, err := ReadTreeDirectory(dir, 1000)
	tree.ApplyLoad(tree.Root, children, cut, err)
	w := &workspace{}
	one := w.NewTab()
	one.Name = "first-tab"
	two := w.NewTab()
	two.Name = "second-tab"
	e := &editorWindow{model: w, sidebar: true, project: tree}
	tt := ui.NewTester(e.view, 1220, 820)
	tt.Frame()
	first, ok := tt.Find("a.go")
	if !ok {
		t.Fatal("short file not visible")
	}
	longer, ok := tt.Find("a_very_long_filename.go")
	if !ok {
		t.Fatal("long file not visible")
	}
	folder, ok := tt.Find("sub")
	if !ok {
		t.Fatal("folder not visible")
	}
	if first.X != longer.X || first.X != folder.X {
		t.Fatalf("same-level labels misaligned: %.2f %.2f %.2f", first.X, longer.X, folder.X)
	}
	if !tt.HasText("first-tab") || !tt.HasText("second-tab") {
		t.Fatal("tabs not rendered")
	}
	if err := tt.Click("first-tab"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if w.ActiveID != one.ID {
		t.Fatal("click did not activate first tab")
	}
	if err := tt.Click("second-tab"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if w.ActiveID != two.ID {
		t.Fatal("click did not activate second tab")
	}
	if err := tt.Click("项目"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if e.sidebar {
		t.Fatal("sidebar did not collapse")
	}
}

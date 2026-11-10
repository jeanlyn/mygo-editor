package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func putFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0640); err != nil {
		t.Fatal(err)
	}
}
func getFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func TestWorkspaceOpenDedupAndSwitch(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "a.go")
	putFile(t, p, "package main\n")
	w := &workspace{}
	a, err := w.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	b := w.NewTab()
	b.Code.Buffer.Set("unsaved")
	a2, err := w.Open(filepath.Join(d, ".", "a.go"))
	if err != nil || a != a2 || len(w.Tabs) != 2 || w.Active() != a {
		t.Fatal("duplicate or inactive", err)
	}
	if !w.Activate(b.ID) || w.Active() != b {
		t.Fatal("switch failed")
	}
	if err = w.Close(b.ID, false); !errors.Is(err, ErrUnsaved) {
		t.Fatal("close lost changes", err)
	}
	if err = w.Close(b.ID, true); err != nil || w.Active() != a {
		t.Fatal(err)
	}
	if err = w.Close(a.ID, false); err != nil || w.Active() != nil {
		t.Fatal(err)
	}
}
func TestTabDirtyUndoAndSaveSnapshot(t *testing.T) {
	w := &workspace{}
	tab := w.NewTab()
	if tab.Dirty() {
		t.Fatal("new tab dirty")
	}
	tab.Code.Buffer.Set("hello")
	if !tab.Dirty() {
		t.Fatal("edit not dirty")
	}
	tab.Code.Buffer.Undo()
	if tab.Dirty() {
		t.Fatal("undo should restore clean")
	}
	tab.Code.Buffer.Redo()
	p := filepath.Join(t.TempDir(), "file")
	req, err := tab.PrepareSave(p, false)
	if err != nil {
		t.Fatal(err)
	}
	tab.Code.Buffer.Set("hello later")
	result, err := saveFile(req)
	if err != nil {
		t.Fatal(err)
	}
	tab.ApplySave(result)
	if !tab.Dirty() || getFile(t, p) != "hello" {
		t.Fatal("concurrent edits marked saved")
	}
	tab.Code.Buffer.Undo()
	if tab.Dirty() {
		t.Fatal("undo to saved snapshot should be clean")
	}
}
func TestSaveConflictAndOverwrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "file")
	putFile(t, p, "old")
	before, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// Windows does not expose Unix permission bits such as 0640.
	wantMode := before.Mode().Perm()
	w := &workspace{}
	tab, err := w.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	tab.Code.Buffer.Set("edited")
	putFile(t, p, "external")
	err = tab.Save("", false)
	var conflict *DiskChangedError
	if !errors.Is(err, ErrDiskChanged) || !errors.As(err, &conflict) || getFile(t, p) != "external" || !tab.Dirty() {
		t.Fatal("conflict not preserved", err)
	}
	if err = tab.Save("", true); err != nil {
		t.Fatal(err)
	}
	if getFile(t, p) != "edited" || tab.Dirty() {
		t.Fatal("overwrite failed")
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != wantMode {
		t.Fatalf("mode %o, want preserved mode %o", st.Mode().Perm(), wantMode)
	}
}
func TestSaveBOMCRLFRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "file")
	putFile(t, p, "\ufeffone\r\ntwo\r\n")
	w := &workspace{}
	tab, err := w.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	tab.Code.Buffer.Set("one\ntwo\nthree\n")
	if err = tab.Save("", false); err != nil {
		t.Fatal(err)
	}
	if getFile(t, p) != "\ufeffone\r\ntwo\r\nthree\r\n" {
		t.Fatal("format changed")
	}
}
func TestSaveAsExistingRequiresOverwrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "file")
	putFile(t, p, "keep")
	tab := (&workspace{}).NewTab()
	tab.Code.Buffer.Set("new")
	if err := tab.Save(p, false); !errors.Is(err, ErrDiskChanged) || getFile(t, p) != "keep" {
		t.Fatal(err)
	}
	if tab.Path != "" {
		t.Fatal("failed save changed path")
	}
	if err := tab.Save(p, true); err != nil {
		t.Fatal(err)
	}
}
func TestOpenSymlinkDedupAndSavePreservesLink(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "real")
	link := filepath.Join(d, "link")
	putFile(t, p, "old")
	if err := os.Symlink(p, link); err != nil {
		t.Skip(err)
	}
	w := &workspace{}
	a, err := w.Open(link)
	if err != nil {
		t.Fatal(err)
	}
	b, err := w.Open(p)
	if err != nil || a != b {
		t.Fatal("symlink not deduped", err)
	}
	a.Code.Buffer.Set("new")
	if err = a.Save("", false); err != nil {
		t.Fatal(err)
	}
	st, err := os.Lstat(link)
	if err != nil || st.Mode()&os.ModeSymlink == 0 || getFile(t, link) != "new" {
		t.Fatal("link replaced", err)
	}
}
func TestDeletedDiskFileIsConflict(t *testing.T) {
	p := filepath.Join(t.TempDir(), "file")
	putFile(t, p, "old")
	w := &workspace{}
	tab, err := w.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err = tab.Save("", false); !errors.Is(err, ErrDiskChanged) {
		t.Fatal(err)
	}
	if _, err = os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("deleted file recreated")
	}
}
func TestInvalidOpenDoesNotAddTab(t *testing.T) {
	for _, text := range []string{"bad\x00", "\xff", "one\r\ntwo\n", "old\rline"} {
		p := filepath.Join(t.TempDir(), "file")
		putFile(t, p, text)
		w := &workspace{}
		if _, err := w.Open(p); err == nil || len(w.Tabs) != 0 {
			t.Fatal("accepted invalid text")
		}
	}
}
func TestFailedSaveKeepsDirtyAndNoTemps(t *testing.T) {
	d := t.TempDir()
	tab := (&workspace{}).NewTab()
	tab.Code.Buffer.Set("new")
	if err := tab.Save(filepath.Join(d, "missing", "file"), false); err == nil {
		t.Fatal("save unexpectedly succeeded")
	}
	if tab.Path != "" || !tab.Dirty() {
		t.Fatal("failure changed state")
	}
	entries, err := os.ReadDir(d)
	if err != nil || len(entries) != 0 {
		t.Fatal("temporary file leaked", err)
	}
}

func TestRejectDirectoryFileOperations(t *testing.T) {
	p := t.TempDir()
	if _, err := readFile(p); err == nil {
		t.Fatal("opened directory")
	}
	if _, _, err := diskStamp(p); err == nil {
		t.Fatal("hashed directory")
	}
	if _, err := saveFile(saveRequest{Path: p, Content: content{Text: "replace", Ending: "\n"}, Overwrite: true}); err == nil {
		t.Fatal("replaced directory")
	}
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		t.Fatal("directory modified", err)
	}
}
func TestRejectOversizedDiskFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "large")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(maxFileSize + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	if _, err = readFile(p); !errors.Is(err, ErrFileTooLarge) {
		t.Fatal("oversized open", err)
	}
	if _, _, err = diskStamp(p); !errors.Is(err, ErrFileTooLarge) {
		t.Fatal("oversized hash", err)
	}
	if _, err = saveFile(saveRequest{Path: p, Content: content{Text: "replace", Ending: "\n"}, Overwrite: true}); !errors.Is(err, ErrFileTooLarge) {
		t.Fatal("oversized overwrite", err)
	}
	st, err := os.Stat(p)
	if err != nil || st.Size() != maxFileSize+1 {
		t.Fatal("oversized target modified", err)
	}
}

func TestHardLinkOpenDedup(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "real")
	alias := filepath.Join(d, "alias")
	putFile(t, p, "same file")
	if err := os.Link(p, alias); err != nil {
		t.Skip(err)
	}
	w := &workspace{}
	first, err := w.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	first.Code.Buffer.Set("unsaved")
	second, err := w.Open(alias)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(w.Tabs) != 1 || w.Active() != first || first.Code.Buffer.String() != "unsaved" {
		t.Fatal("hard link created duplicate tab or lost edits")
	}
	if first.Identity == nil {
		t.Fatal("file identity absent")
	}
}

func TestSaveRefreshesFileIdentity(t *testing.T) {
	p := filepath.Join(t.TempDir(), "file")
	putFile(t, p, "old")
	w := &workspace{}
	tab, err := w.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	oldIdentity := tab.Identity
	tab.Code.Buffer.Set("saved")
	if err = tab.Save("", false); err != nil {
		t.Fatal(err)
	}
	current, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if tab.Identity == nil || !os.SameFile(tab.Identity, current) {
		t.Fatal("saved identity stale")
	}
	if os.SameFile(oldIdentity, tab.Identity) {
		t.Fatal("expected atomic replacement inode")
	}
}

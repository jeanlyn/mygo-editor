package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui/editor"
)

var ErrDiskChanged = errors.New("磁盘上的文件已更改")
var ErrUnsaved = errors.New("标签页有未保存的更改")
var ErrTabNotFound = errors.New("标签页不存在")
var ErrFileTooLarge = errors.New("文件超过 16 MiB，无法安全打开或保存")

// DiskChangedError supports errors.Is(err, ErrDiskChanged).
type DiskChangedError struct{ Path string }

func (e *DiskChangedError) Error() string { return fmt.Sprintf("%s：%s", ErrDiskChanged, e.Path) }
func (e *DiskChangedError) Unwrap() error { return ErrDiskChanged }

type fileStamp struct {
	Exists bool
	Hash   [32]byte
}
type loadedFile struct {
	Path, Name string
	Content    content
	Stamp      fileStamp
	Identity   os.FileInfo
}
type editorTab struct {
	ID         uint64
	Path, Name string
	Code       *editor.State
	Language   string
	Wrap       bool
	Identity   os.FileInfo
	saved      content
	stamp      fileStamp
}
type workspace struct {
	Tabs     []*editorTab
	ActiveID uint64
	nextID   uint64
}

func (w *workspace) add(v content, path string, stamp fileStamp) *editorTab {
	w.nextID++
	name := "未命名"
	if path != "" {
		name = filepath.Base(path)
	}
	t := &editorTab{ID: w.nextID, Path: path, Name: name, Code: newEditor(v), Language: "自动", saved: v, stamp: stamp}
	w.Tabs = append(w.Tabs, t)
	w.ActiveID = t.ID
	return t
}
func (w *workspace) NewTab() *editorTab { return w.add(content{Ending: "\n"}, "", fileStamp{}) }
func (w *workspace) Active() *editorTab {
	for _, t := range w.Tabs {
		if t.ID == w.ActiveID {
			return t
		}
	}
	return nil
}
func (w *workspace) Activate(id uint64) bool {
	for _, t := range w.Tabs {
		if t.ID == id {
			w.ActiveID = id
			return true
		}
	}
	return false
}
func (w *workspace) ApplyOpen(f loadedFile) *editorTab {
	for _, t := range w.Tabs {
		if t.Path == f.Path || (t.Identity != nil && f.Identity != nil && os.SameFile(t.Identity, f.Identity)) {
			w.ActiveID = t.ID
			return t
		}
	}
	t := w.add(f.Content, f.Path, f.Stamp)
	t.Identity = f.Identity
	return t
}
func (w *workspace) Open(path string) (*editorTab, error) {
	f, err := readFile(path)
	if err != nil {
		return nil, err
	}
	return w.ApplyOpen(f), nil
}
func (w *workspace) Close(id uint64, discard bool) error {
	for i, t := range w.Tabs {
		if t.ID != id {
			continue
		}
		if t.Dirty() && !discard {
			return ErrUnsaved
		}
		w.Tabs = append(w.Tabs[:i], w.Tabs[i+1:]...)
		if w.ActiveID == id {
			w.ActiveID = 0
			if len(w.Tabs) > 0 {
				if i >= len(w.Tabs) {
					i = len(w.Tabs) - 1
				}
				w.ActiveID = w.Tabs[i].ID
			}
		}
		return nil
	}
	return ErrTabNotFound
}
func (t *editorTab) Snapshot() content {
	return content{Text: t.Code.Buffer.String(), Ending: t.Code.Buffer.LineEnding(), BOM: t.saved.BOM}
}
func (t *editorTab) Dirty() bool { return t.Snapshot() != t.saved }

// canonicalPath follows existing symlinks, including parent-directory symlinks.
// A dangling link is rejected instead of replacing the link itself.
func canonicalPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("请选择保存路径")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if _, linkErr := os.Lstat(abs); linkErr == nil {
		return "", err
	} else if !os.IsNotExist(linkErr) {
		return "", linkErr
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}
func readFile(path string) (loadedFile, error) {
	p, err := canonicalPath(path)
	if err != nil {
		return loadedFile{}, err
	}
	info, err := os.Stat(p)
	if err != nil {
		return loadedFile{}, err
	}
	if !info.Mode().IsRegular() {
		return loadedFile{}, errors.New("只能打开普通文本文件")
	}
	if info.Size() > maxFileSize {
		return loadedFile{}, ErrFileTooLarge
	}
	f, err := os.Open(p)
	if err != nil {
		return loadedFile{}, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return loadedFile{}, err
	}
	if !info.Mode().IsRegular() {
		return loadedFile{}, errors.New("只能打开普通文本文件")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return loadedFile{}, err
	}
	v, err := readContent(bytes.NewReader(data))
	if err != nil {
		return loadedFile{}, err
	}
	return loadedFile{Path: p, Name: filepath.Base(p), Content: v, Identity: info, Stamp: fileStamp{Exists: true, Hash: sha256.Sum256(data)}}, nil
}
func diskStamp(path string) (fileStamp, os.FileMode, error) {
	// Stat before opening: opening a FIFO can block indefinitely before f.Stat.
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return fileStamp{}, 0600, nil
	}
	if err != nil {
		return fileStamp{}, 0, err
	}
	if !st.Mode().IsRegular() {
		return fileStamp{}, 0, errors.New("保存目标不是普通文件")
	}
	if st.Size() > maxFileSize {
		return fileStamp{}, 0, ErrFileTooLarge
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return fileStamp{}, 0600, nil
	}
	if err != nil {
		return fileStamp{}, 0, err
	}
	defer f.Close()
	st, err = f.Stat()
	if err != nil {
		return fileStamp{}, 0, err
	}
	if !st.Mode().IsRegular() {
		return fileStamp{}, 0, errors.New("保存目标不是普通文件")
	}
	if st.Size() > maxFileSize {
		return fileStamp{}, 0, ErrFileTooLarge
	}
	h := sha256.New()
	// Bound hashing even if another process grows the file after Stat.
	n, err := io.Copy(h, io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return fileStamp{}, 0, err
	}
	if n > maxFileSize {
		return fileStamp{}, 0, ErrFileTooLarge
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return fileStamp{Exists: true, Hash: sum}, st.Mode().Perm(), nil
}

type saveRequest struct {
	Path      string
	Content   content
	Expected  fileStamp
	Overwrite bool
}
type saveResult struct {
	Identity os.FileInfo
	Path     string
	Content  content
	Stamp    fileStamp
}

// PrepareSave snapshots UI-owned state only; all filesystem work is in saveFile.
func (t *editorTab) PrepareSave(path string, overwrite bool) (saveRequest, error) {
	if path == "" {
		path = t.Path
	}
	if path == "" {
		return saveRequest{}, errors.New("请选择保存路径")
	}
	expected := fileStamp{}
	if path == t.Path {
		expected = t.stamp
	}
	return saveRequest{Path: path, Content: t.Snapshot(), Expected: expected, Overwrite: overwrite}, nil
}
func saveFile(req saveRequest) (saveResult, error) {
	p, err := canonicalPath(req.Path)
	if err != nil {
		return saveResult{}, err
	}
	before, mode, err := diskStamp(p)
	if err != nil {
		return saveResult{}, err
	}
	if !req.Overwrite && before != req.Expected {
		return saveResult{}, &DiskChangedError{p}
	}
	var data bytes.Buffer
	if err = writeContent(&data, req.Content); err != nil {
		return saveResult{}, err
	}
	if data.Len() > maxFileSize {
		return saveResult{}, ErrFileTooLarge
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".mygo-save-*")
	if err != nil {
		return saveResult{}, err
	}
	tempPath := tmp.Name()
	defer os.Remove(tempPath)
	if _, err = tmp.Write(data.Bytes()); err == nil {
		err = tmp.Chmod(mode)
	}
	if err == nil {
		err = tmp.Sync()
	}
	// Capture the replacement inode after writing, before rename, so a Stat
	// failure cannot be reported after the original file has been replaced.
	var identity os.FileInfo
	if err == nil {
		identity, err = tmp.Stat()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return saveResult{}, err
	}
	// Recheck immediately before atomic replacement, including symlink changes.
	nowPath, err := canonicalPath(req.Path)
	if err != nil {
		return saveResult{}, err
	}
	if nowPath != p {
		return saveResult{}, &DiskChangedError{req.Path}
	}
	now, _, err := diskStamp(p)
	if err != nil {
		return saveResult{}, err
	}
	if now != before {
		return saveResult{}, &DiskChangedError{p}
	}
	// Atomic replacement preserves permission bits, not hard-link identity or
	// extended metadata (ACLs/xattrs). Other hard links keep the prior contents.
	if err = os.Rename(tempPath, p); err != nil {
		return saveResult{}, err
	}
	return saveResult{Path: p, Content: req.Content, Identity: identity, Stamp: fileStamp{Exists: true, Hash: sha256.Sum256(data.Bytes())}}, nil
}

// ApplySave records the exact saved snapshot, so typing during save stays dirty.
func (t *editorTab) ApplySave(r saveResult) {
	t.Path = r.Path
	t.Name = filepath.Base(r.Path)
	t.saved = r.Content
	t.stamp = r.Stamp
	t.Identity = r.Identity
}
func (t *editorTab) Save(path string, overwrite bool) error {
	r, err := t.PrepareSave(path, overwrite)
	if err != nil {
		return err
	}
	result, err := saveFile(r)
	if err == nil {
		t.ApplySave(result)
	}
	return err
}

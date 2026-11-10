package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type editorWindow struct {
	win               *mygo.Window
	model             *workspace
	sidebar           bool
	busy              bool
	closingApproved   bool
	title             string
	project           *ProjectTree
	projectGeneration uint64
}

var appWindow *editorWindow

func (e *editorWindow) invalidate() { e.win.Invalidate() }
func (e *editorWindow) action(fn func()) {
	if e.busy {
		return
	}
	e.busy = true
	for _, t := range e.model.Tabs {
		t.Code.ReadOnly = true
	}
	e.invalidate()
	go func() {
		defer mygo.RunOnMain(func() {
			e.busy = false
			for _, t := range e.model.Tabs {
				t.Code.ReadOnly = false
			}
			e.invalidate()
		})
		fn()
	}()
}
func (e *editorWindow) report(err error) {
	if err == nil || errors.Is(err, mygo.ErrCanceled) {
		return
	}
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: e.win, Type: mygo.MessageError, Title: "操作未完成", Message: err.Error()})
}
func (e *editorWindow) newTab() {
	if e.busy {
		return
	}
	e.model.NewTab()
	e.invalidate()
}
func (e *editorWindow) command(cmd editor.Command) {
	if e.busy {
		return
	}
	if t := e.model.Active(); t != nil {
		e.win.Update(func() {
			if !e.busy {
				t.Code.Run(cmd)
			}
		})
	}
}

func (e *editorWindow) saveTab(t *editorTab, as bool) bool {
	path := t.Path
	if as || path == "" {
		var err error
		path, err = mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: e.win, Title: "保存文件", DefaultPath: t.Name})
		if err != nil {
			e.report(err)
			return false
		}
		if path == "" {
			return false
		}
	}
	target, pathErr := canonicalPath(path)
	if pathErr != nil {
		e.report(pathErr)
		return false
	}
	targetInfo, _ := os.Stat(target)
	var duplicate bool
	mygo.RunOnMain(func() {
		for _, other := range e.model.Tabs {
			if other != t && (other.Path == target || (targetInfo != nil && other.Identity != nil && os.SameFile(targetInfo, other.Identity))) {
				duplicate = true
			}
		}
	})
	if duplicate {
		e.report(fmt.Errorf("目标已在另一个标签中打开，请切换到该标签或使用其他文件名"))
		return false
	}
	var req saveRequest
	var err error
	mygo.RunOnMain(func() { req, err = t.PrepareSave(target, false) })
	if err != nil {
		e.report(err)
		return false
	}
	result, err := saveFile(req)
	if errors.Is(err, ErrDiskChanged) {
		answer, aerr := mygo.Dialog.Message(mygo.MessageOptions{Parent: e.win, Type: mygo.MessageWarning, Title: "文件已改变", Message: "磁盘文件已被其他程序修改，或目标文件已经存在。", Detail: "覆盖会替换磁盘上的内容。你也可以取消后另存为。", Buttons: []string{"取消", "覆盖"}, DefaultButton: 0, CancelButton: 0, DestructiveButtons: []int{1}})
		if aerr != nil || answer.Button != 1 {
			return false
		}
		mygo.RunOnMain(func() { req, err = t.PrepareSave(target, true) })
		if err == nil {
			result, err = saveFile(req)
		}
	}
	if err != nil {
		e.report(err)
		return false
	}
	mygo.RunOnMain(func() { t.ApplySave(result); e.invalidate() })
	return true
}
func (e *editorWindow) save(as bool) {
	t := e.model.Active()
	if t == nil {
		return
	}
	e.action(func() { e.saveTab(t, as) })
}
func (e *editorWindow) mayClose(t *editorTab) (content, bool) {
	var snapshot content
	var dirty bool
	mygo.RunOnMain(func() { snapshot = t.Snapshot(); dirty = t.Dirty() })
	if !dirty {
		return snapshot, true
	}
	answer, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: e.win, Type: mygo.MessageWarning, Title: "保存更改？", Message: "“" + t.Name + "”有未保存的修改。", Buttons: []string{"取消", "不保存", "保存"}, DefaultButton: 2, CancelButton: 0, DestructiveButtons: []int{1}})
	if err != nil {
		e.report(err)
		return content{}, false
	}
	switch answer.Button {
	case 1:
		return snapshot, true
	case 2:
		if !e.saveTab(t, false) {
			return content{}, false
		}
		mygo.RunOnMain(func() { snapshot = t.Snapshot(); dirty = t.Dirty() })
		return snapshot, !dirty
	default:
		return content{}, false
	}
}
func (e *editorWindow) closeTab(t *editorTab) {
	e.action(func() {
		snapshot, ok := e.mayClose(t)
		if !ok {
			return
		}
		mygo.RunOnMain(func() {
			if t.Snapshot() != snapshot {
				return
			} // Never discard an edit arriving after the decision.
			_ = e.model.Close(t.ID, true)
			if len(e.model.Tabs) == 0 {
				e.model.NewTab()
			}
			e.invalidate()
		})
	})
}
func (e *editorWindow) requestClose(quit bool) {
	if e.busy {
		return
	}
	tabs := append([]*editorTab(nil), e.model.Tabs...)
	e.action(func() {
		approved := make(map[*editorTab]content, len(tabs))
		for _, t := range tabs {
			snapshot, ok := e.mayClose(t)
			if !ok {
				return
			}
			approved[t] = snapshot
		}
		mygo.RunOnMain(func() {
			for _, t := range tabs {
				if t.Snapshot() != approved[t] {
					return
				}
			}
			e.closingApproved = true
			if quit {
				mygo.App.Quit()
			} else {
				e.win.Close()
			}
		})
	})
}
func (e *editorWindow) openPath(path string) { e.action(func() { e.loadPath(path) }) }
func (e *editorWindow) loadPath(path string) {
	loaded, err := readFile(path)
	if err != nil {
		e.report(err)
		return
	}
	mygo.RunOnMain(func() {
		if len(e.model.Tabs) == 1 {
			first := e.model.Tabs[0]
			if first.Path == "" && !first.Dirty() && first.Code.Buffer.Len() == 0 {
				_ = e.model.Close(first.ID, true)
			}
		}
		e.model.ApplyOpen(loaded)
		e.invalidate()
	})
}
func (e *editorWindow) openFiles() {
	e.action(func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: e.win, Title: "打开文件", Multiple: true})
		if err != nil {
			e.report(err)
			return
		}
		for _, p := range paths {
			e.loadPath(p)
		}
	})
}

func menuItem(label, key string, fn func()) *mygo.MenuItem {
	return &mygo.MenuItem{Label: label, Accelerator: key, Click: func(_ *mygo.MenuItem, _ *mygo.Window) { fn() }}
}
func (e *editorWindow) setupMenu() {
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			menuItem("新建文件", "CmdOrCtrl+N", e.newTab), menuItem("打开文件…", "CmdOrCtrl+O", e.openFiles), menuItem("打开文件夹…", "CmdOrCtrl+Shift+O", e.openFolder),
			{Type: mygo.MenuItemSeparator}, menuItem("保存", "CmdOrCtrl+S", func() { e.save(false) }), menuItem("另存为…", "CmdOrCtrl+Shift+S", func() { e.save(true) }),
			{Type: mygo.MenuItemSeparator}, menuItem("关闭标签", "CmdOrCtrl+W", func() {
				if t := e.model.Active(); t != nil {
					e.closeTab(t)
				}
			}), {Label: "退出", Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleEditMenu},
		{Label: "代码", Submenu: []*mygo.MenuItem{
			menuItem("查找", "CmdOrCtrl+F", func() { e.command(editor.Find) }), menuItem("替换", "CmdOrCtrl+Alt+F", func() { e.command(editor.FindReplace) }), menuItem("跳转行", "Ctrl+G", func() { e.command(editor.GoToLine) }),
			menuItem("切换行注释", "CmdOrCtrl+/", func() { e.command(editor.ToggleComment) }), menuItem("缩进", "CmdOrCtrl+]", func() { e.command(editor.Indent) }), menuItem("减少缩进", "CmdOrCtrl+[", func() { e.command(editor.Outdent) }),
		}},
		{Label: "视图", Submenu: []*mygo.MenuItem{menuItem("切换项目树", "CmdOrCtrl+B", func() { e.sidebar = !e.sidebar; e.invalidate() }), menuItem("下一个标签", "Ctrl+Tab", func() { e.cycleTab(1) }), menuItem("上一个标签", "Ctrl+Shift+Tab", func() { e.cycleTab(-1) })}},
		{Role: mygo.RoleWindowMenu},
	}))
}
func (e *editorWindow) cycleTab(delta int) {
	for i, t := range e.model.Tabs {
		if t.ID == e.model.ActiveID {
			n := (i + delta + len(e.model.Tabs)) % len(e.model.Tabs)
			e.model.Activate(e.model.Tabs[n].ID)
			e.invalidate()
			return
		}
	}
}

func main() {
	mygo.App.SetName("MyGo Editor")
	mygo.Theme.SetSource(mygo.ThemeDark)
	mygo.App.WhenReady(func() {
		e := &editorWindow{model: &workspace{}, sidebar: true}
		appWindow = e
		e.model.NewTab()
		e.win = mygo.NewWindow(mygo.WindowOptions{Title: "MyGo Editor", Width: 1220, Height: 820, BackgroundColor: "#1f2329", StateKey: "workspace", Content: ui.View(e.view)})
		e.setupMenu()
		e.win.OnClose(func(ev *mygo.CloseEvent) {
			if e.closingApproved {
				return
			}
			ev.PreventDefault()
			e.requestClose(false)
		})
		mygo.App.OnBeforeQuit(func(ev *mygo.QuitEvent) {
			if e.closingApproved {
				return
			}
			ev.PreventDefault()
			e.requestClose(true)
		})
		if len(os.Args) > 1 {
			e.action(func() {
				for _, arg := range os.Args[1:] {
					p, err := filepath.Abs(arg)
					if err != nil {
						e.report(err)
						continue
					}
					info, err := os.Stat(p)
					if err != nil {
						e.report(err)
						continue
					}
					if info.IsDir() {
						mygo.RunOnMain(func() { e.setProject(p) })
					} else {
						e.loadPath(p)
					}
				}
			})
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

var languageOptions = []string{"自动", "纯文本", "Go", "JavaScript", "TypeScript", "JSON", "Python", "Rust", "C", "C++", "Java", "Shell", "YAML", "TOML", "SQL", "HTML", "CSS", "Markdown"}

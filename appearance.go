package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// Quiet, editor-first chrome inspired by Zed's classic workspace.
// This is an independent palette, not a Zed theme or brand asset.
var chromeTheme = func() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = ui.Hex("#1f2329")
	t.Surface = ui.Hex("#252a32")
	t.SurfaceHover = ui.Hex("#303742")
	t.SurfacePressed = ui.Hex("#394455")
	t.Border = ui.Hex("#333a45")
	t.Text = ui.Hex("#d7dce4")
	t.TextMuted = ui.Hex("#929baa")
	t.Accent = ui.Hex("#88b4ed")
	t.Selection = ui.Hex("#354a65")
	t.Radius = 4
	t.Spacing = 3
	t.FontSize = 12
	return t
}()

var codeTheme = func() *editor.Theme {
	t := editor.DefaultTheme(chromeTheme)
	t.Background = chromeTheme.Background
	t.ActiveLine = ui.Hex("#252c35")
	t.LineNumber = ui.Hex("#657081")
	t.ActiveLineNumber = ui.Hex("#bdc7d5")
	t.IndentGuide = ui.Hex("#303741")
	t.WrapGuide = ui.Hex("#2a3039")
	for k, c := range map[editor.TokenKind]string{
		editor.TokenKeyword: "#c6a5e8", editor.TokenOperator: "#abb8ca", editor.TokenType: "#e2bc87",
		editor.TokenFunction: "#8fb9ed", editor.TokenString: "#a5c698", editor.TokenNumber: "#d9ac81",
		editor.TokenComment: "#7f8999", editor.TokenProperty: "#b4c8de", editor.TokenConstant: "#d9ac81",
	} {
		t.Tokens[k].Color = ui.Hex(c)
	}
	return t
}()

func quietButton(c *ui.Context, label string) ui.Element {
	b := ui.ButtonBase(c).Height(28).Padding(0, 10).Radius(4)
	if b.Hovered() {
		b = b.Background(chromeTheme.SurfaceHover)
	}
	b.Children(func() { ui.Text(c, label).FontSize(12).TextColor(chromeTheme.TextMuted) })
	return b
}

func (e *editorWindow) view(c *ui.Context) {
	c.SetTheme(chromeTheme)
	th := c.Theme()
	tab := e.model.Active()
	if tab == nil {
		return
	}
	st := tab.Code
	st.SoftWrap = tab.Wrap
	st.Theme = codeTheme
	st.Font = ui.Font{Family: "monospace", Size: 14}
	st.LineHeight = 1.6
	st.ReadOnly = e.busy
	if tab.Language == "自动" {
		st.Buffer.SetLanguage(editor.LanguageFor(tab.Path))
	} else if tab.Language == "纯文本" {
		st.Buffer.SetLanguage(nil)
	} else {
		st.Buffer.SetLanguage(editor.LanguageNamed(tab.Language))
	}
	title := tab.Name + " - MyGo Editor"
	if tab.Dirty() {
		title = "● " + title
	}
	if title != e.title {
		e.title = title
		if e.win != nil {
			e.win.SetTitle(title)
		}
	}
	ui.Column(c).Fill().Background(th.Background).Children(func() {
		ui.Row(c).Height(39).Padding(0, 12).Gap(3).AlignItems(ui.Center).Background(th.Surface).BorderWidth(0, 0, 1, 0).BorderColor(th.Border).Children(func() {
			ui.Text(c, "MYGO").FontSize(11).Bold().TextColor(th.Accent).Width(57)
			if quietButton(c.Key("sidebar"), "项目").Clicked() {
				e.sidebar = !e.sidebar
			}
			ui.Box(c).Width(1).Height(15).Background(th.Border)
			if quietButton(c.Key("new"), "新建").Clicked() {
				e.newTab()
			}
			if quietButton(c.Key("open"), "打开文件").Clicked() {
				e.openFiles()
			}
			if quietButton(c.Key("folder"), "打开文件夹").Clicked() {
				e.openFolder()
			}
			if quietButton(c.Key("save"), "保存").Clicked() {
				e.save(false)
			}
			ui.Box(c).Grow(1)
			if quietButton(c.Key("find"), "查找").Clicked() {
				e.command(editor.Find)
			}
			if quietButton(c.Key("replace"), "替换").Clicked() {
				e.command(editor.FindReplace)
			}
			if quietButton(c.Key("goto"), "跳转行").Clicked() {
				e.command(editor.GoToLine)
			}
		})
		ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
			if e.sidebar {
				e.projectView(c)
			}
			ui.Column(c).Grow(1).FillHeight().Children(func() {
				ui.ScrollHorizontal(c.Key("tabs")).Height(39).FillWidth().Background(th.Surface).Children(func() {
					ui.Row(c).Height(39).Children(func() {
						for _, item := range e.model.Tabs {
							t := item
							active := t.ID == e.model.ActiveID
							ui.Row(c.Key(fmt.Sprint(t.ID))).Height(39).Padding(0, 4).Gap(2).Background(func() ui.Color {
								if active {
									return th.Background
								}
								return th.Surface
							}()).BorderWidth(0, 0, 2, 0).BorderColor(func() ui.Color {
								if active {
									return th.Accent
								}
								return th.Surface
							}()).Children(func() {
								b := ui.ButtonBase(c.Key("select")).Height(37).Padding(0, 12).Gap(8)
								b.Children(func() {
									ui.Text(c, t.Name).FontSize(12).TextColor(th.Text)
									if t.Dirty() {
										ui.Text(c, "●").FontSize(8).TextColor(th.Accent)
									}
								})
								if b.Clicked() {
									e.model.Activate(t.ID)
								}
								if quietButton(c.Key("close"), "×").Width(25).Clicked() {
									e.closeTab(t)
								}
							})
						}
					})
				})
				crumb := "未保存的新文件"
				if tab.Path != "" {
					crumb = tab.Path
					if e.project != nil {
						if rel, err := filepath.Rel(e.project.Root.Path, tab.Path); err == nil && !strings.HasPrefix(rel, "..") {
							crumb = e.project.Root.Name + "  /  " + strings.ReplaceAll(rel, string(filepath.Separator), "  /  ")
						}
					}
				}
				ui.Text(c, crumb).Height(31).Padding(7, 18).FontSize(11).TextColor(th.TextMuted).Clip()
				editor.View(c.Key(fmt.Sprintf("editor-%d", tab.ID)), st).Grow(1).FillWidth().AutoFocus()
			})
		})
		ui.Row(c).Height(31).Padding(0, 12).Gap(16).AlignItems(ui.Center).Background(th.Surface).BorderWidth(1, 0, 0, 0).BorderColor(th.Border).Children(func() {
			status := "已保存"
			if tab.Path == "" {
				status = "尚未保存"
			} else if tab.Dirty() {
				status = "未保存修改"
			}
			if e.busy {
				status = "处理中 · 暂停编辑"
			}
			ui.Text(c, "●").FontSize(8).TextColor(th.Accent)
			ui.Text(c, status).FontSize(11).TextColor(th.TextMuted)
			ui.Text(c, fmt.Sprintf("%d 个标签", len(e.model.Tabs))).FontSize(11).TextColor(th.TextMuted)
			ui.Box(c).Grow(1)
			pos := st.CursorPos()
			line := st.Buffer.Line(pos.Line)
			col := utf8.RuneCountInString(line[:min(pos.Col, len(line))]) + 1
			ui.Text(c, fmt.Sprintf("行 %d，列 %d", pos.Line+1, col)).FontSize(11).TextColor(th.TextMuted)
			ending := "LF"
			if st.Buffer.LineEnding() == "\r\n" {
				ending = "CRLF"
			}
			ui.Text(c, "UTF-8 · "+ending).FontSize(11).TextColor(th.TextMuted)
			label := "换行: 关"
			if tab.Wrap {
				label = "换行: 开"
			}
			if quietButton(c.Key("wrap"), label).Clicked() {
				tab.Wrap = !tab.Wrap
			}
			ui.Select(c, &tab.Language, languageOptions).Width(110).Height(25).Label("语法高亮语言")
		})
	})
}

func (e *editorWindow) openFolder() {
	e.action(func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: e.win, Title: "打开项目文件夹", Directory: true})
		if err != nil {
			e.report(err)
			return
		}
		if len(paths) > 0 {
			mygo.RunOnMain(func() { e.setProject(paths[0]) })
		}
	})
}
func (e *editorWindow) setProject(path string) {
	e.projectGeneration++
	generation := e.projectGeneration
	// Root validation is performed off the GUI thread too.
	go func() {
		tree, err := NewProjectTree(path)
		if err != nil {
			e.report(err)
			return
		}
		mygo.RunOnMain(func() {
			if generation != e.projectGeneration {
				return
			}
			e.project = tree
			e.sidebar = true
			e.loadNode(tree, tree.Root)
			e.invalidate()
		})
	}()
}
func (e *editorWindow) loadNode(tree *ProjectTree, node *treeNode) {
	if !tree.BeginLoad(node) {
		return
	}
	e.invalidate()
	go func() {
		children, truncated, err := ReadTreeDirectory(node.Path, DefaultTreeEntryLimit)
		mygo.RunOnMain(func() {
			if e.project != tree {
				return
			}
			tree.ApplyLoad(node, children, truncated, err)
			e.invalidate()
		})
	}()
}
func (e *editorWindow) projectView(c *ui.Context) {
	th := c.Theme()
	ui.Column(c).Width(230).Shrink(0).Background(ui.Hex("#23272e")).BorderWidth(0, 1, 0, 0).BorderColor(th.Border).Children(func() {
		ui.Row(c).Height(42).Padding(0, 14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "项目文件").FontSize(11).TextColor(th.TextMuted).Grow(1)
			if quietButton(c.Key("refresh"), "刷新").Clicked() && e.project != nil {
				e.setProject(e.project.Root.Path)
			}
		})
		if e.project == nil {
			ui.Column(c).Padding(16).Gap(12).Children(func() {
				ui.Text(c, "打开文件夹以浏览项目").FontSize(12).TextColor(th.TextMuted)
				if quietButton(c.Key("openroot"), "打开文件夹…").Clicked() {
					e.openFolder()
				}
			})
			ui.Box(c).Grow(1)
		} else {
			tree := e.project
			ui.Scroll(c).Grow(1).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Padding(6).Gap(1).Children(func() {
					for _, row := range tree.VisibleRows() {
						n, depth := row.Node, row.Depth
						b := ui.ButtonBase(c.Key(n.Path)).Height(28).FillWidth().Justify(ui.Start).Padding(0, 8+float32(depth)*16).Gap(7).Radius(3).Clip()
						if t := e.model.Active(); t != nil && t.Path == n.Path {
							b = b.Background(ui.Hex("#323c4b"))
						} else if b.Hovered() {
							b = b.Background(th.SurfaceHover)
						}
						b.Children(func() {
							mark := "·"
							if n.IsDir {
								mark = "›"
								if n.Expanded {
									mark = "⌄"
								}
							} else if n.Symlink {
								mark = "↗"
							}
							ui.Text(c, mark).Width(16).Shrink(0).FontSize(12).TextColor(th.Accent)
							ui.Text(c, n.Name).Grow(1).Clip().FontSize(12).TextColor(th.Text)
						})
						if b.Clicked() {
							if n.IsDir {
								tree.SetExpanded(n, !n.Expanded)
								if n.Expanded && !n.Loaded {
									e.loadNode(tree, n)
								}
							} else {
								e.openPath(n.Path)
							}
						}
						if n.Loading {
							ui.Text(c, "加载中…").PaddingX(18 + float32(depth)*13).FontSize(10).TextColor(th.TextMuted)
						}
						if n.Err != nil {
							ui.Text(c, "无法读取目录（可刷新重试）").FontSize(10).TextColor(th.Warning)
						}
						if n.Expanded && n.Truncated {
							ui.Text(c, "仅显示前 1000 项").FontSize(10).TextColor(th.Warning)
						}
					}
				})
			})
		}
		ui.Column(c).Padding(14).Gap(7).BorderWidth(1, 0, 0, 0).BorderColor(th.Border).Children(func() {
			ui.Text(c, "本地工作区").FontSize(11).TextColor(th.TextMuted)
			ui.Text(c, "Ctrl / ⌘ + S  保存").FontSize(11).TextColor(th.TextMuted)
			ui.Text(c, "Ctrl + Tab  切换标签").FontSize(11).TextColor(th.TextMuted)
		})
	})
}

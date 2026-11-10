package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestReadWriteRoundTrip(t *testing.T) {
	for _, input := range []string{"", "hello", "你好\n世界\n", "a\r\nb\r\n", "\ufeffpackage main\n", "\ufeffa\r\nb"} {
		t.Run(input, func(t *testing.T) {
			v, err := readContent(strings.NewReader(input))
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := writeContent(&out, v); err != nil {
				t.Fatal(err)
			}
			if out.String() != input {
				t.Fatalf("round trip changed bytes: %q != %q", out.String(), input)
			}
		})
	}
}
func TestRejectUnsafeInput(t *testing.T) {
	for _, input := range []string{"a\x00b", "\xff", "a\rb", "a\r\nb\n", strings.Repeat("x", maxFileSize+1)} {
		if _, err := readContent(strings.NewReader(input)); err == nil {
			t.Fatal("expected input rejection")
		}
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }
func TestWriteError(t *testing.T) {
	if writeContent(failedWriter{}, content{Text: "x"}) == nil {
		t.Fatal("lost write error")
	}
}
func TestAllListedLanguagesExist(t *testing.T) {
	for _, name := range languageOptions[2:] {
		if editor.LanguageNamed(name) == nil {
			t.Fatalf("missing language %s", name)
		}
	}
}
func TestEditorEditing(t *testing.T) {
	st := newEditor(content{Text: "package main\n", Ending: "\n"})
	st.Buffer.SetLanguage(editor.LanguageFor("main.go"))
	tt := ui.NewTester(func(c *ui.Context) { editor.View(c, st).Fill().AutoFocus() }, 800, 600)
	tt.Frame()
	st.GoTo(editor.Pos{Line: 1})
	st.Insert("// 你好")
	tt.Frame()
	if !strings.Contains(st.Buffer.String(), "// 你好") {
		t.Fatal("unicode insertion failed")
	}
	if !st.Run(editor.Undo) {
		t.Fatal("undo failed")
	}
	tt.Frame()
	if st.Buffer.String() != "package main\n" {
		t.Fatalf("undo: %q", st.Buffer.String())
	}
	if !st.Run(editor.Redo) {
		t.Fatal("redo failed")
	}
	tt.Frame()
	st.Find("你好")
	tt.Frame()
	n, _ := st.FindMatches()
	if n != 1 {
		t.Fatalf("find matches: %d", n)
	}
	st.GoTo(editor.Pos{Line: 0})
	st.Run(editor.ToggleComment)
	tt.Frame()
	if !strings.HasPrefix(st.Buffer.String(), "//") {
		t.Fatal("line comment failed")
	}
	st.SoftWrap = true
	tt.Frame()
}

package main

import (
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

const maxFileSize = 16 << 20

// Immutable values let the document service save while typing continues.
type content struct {
	Text   string
	Ending string
	BOM    bool
}

func readContent(r io.Reader) (content, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxFileSize+1))
	if err != nil {
		return content{}, err
	}
	if len(data) > maxFileSize {
		return content{}, errors.New("文件超过 16 MiB，本编辑器暂不打开超大文件")
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return content{}, errors.New("只支持 UTF-8 文本；请先转换编码，不能打开二进制文件")
	}
	s := string(data)
	bom := strings.HasPrefix(s, "\ufeff")
	s = strings.TrimPrefix(s, "\ufeff")
	// Reject mixed/lone-CR endings rather than silently rewriting the file.
	withoutCRLF := strings.ReplaceAll(s, "\r\n", "")
	if strings.Contains(withoutCRLF, "\r") || (strings.Contains(s, "\r\n") && strings.Contains(withoutCRLF, "\n")) {
		return content{}, errors.New("暂不支持混合换行或旧式 CR 换行；请先统一为 LF 或 CRLF")
	}
	ending := "\n"
	if strings.Contains(s, "\r\n") {
		ending = "\r\n"
	}
	return content{Text: strings.ReplaceAll(s, "\r\n", "\n"), Ending: ending, BOM: bom}, nil
}

func writeContent(w io.Writer, v content) error {
	text := v.Text
	if v.Ending == "\r\n" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if v.BOM {
		text = "\ufeff" + text
	}
	_, err := io.WriteString(w, text)
	return err
}

func newEditor(v content) *editor.State {
	b := editor.NewBuffer(v.Text)
	b.SetLineEnding(v.Ending)
	return &editor.State{Buffer: b, WrapGuides: []int{100}, Label: "代码编辑器", Placeholder: "开始编写代码，或打开文件…"}
}

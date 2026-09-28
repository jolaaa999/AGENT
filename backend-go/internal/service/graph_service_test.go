package service

import "testing"

// 首行是空标题（如 "#\n\n正文"）时，兜底文件名不能退化成 ".md"。
// 曾出现用户笔记被命名为隐藏文件 ".md" 的情况。
func TestDeriveFileNameSkipsEmptyHeadings(t *testing.T) {
	cases := map[string]string{
		"# \n\n## 水平运输系统\n内容":     "水平运输系统",
		"#\n\n# 真实标题\n内容":         "真实标题",
		"#\n\n正文第一行":              "正文第一行",
		"###  \n\n  \n\n**加粗标题**": "加粗标题",
	}
	for input, want := range cases {
		got := deriveFileName(input)
		if got != want {
			t.Errorf("deriveFileName(%q) = %q, want %q", input, got, want)
		}
	}
	// 纯空白输入仍要有可读兜底
	if got := deriveFileName("   \n\n#  \n"); got == "" || got == "#" || got == ".md" {
		t.Errorf("blank markdown produced bad name %q", got)
	}
}

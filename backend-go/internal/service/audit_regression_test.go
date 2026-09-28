package service

import (
	"strings"
	"testing"
)

// TestSanitizeFileNameRejectsTraversal 覆盖工作区沙箱最关键的一层防线。
//
// 这些用例在修复前后都应通过（属于既有安全保证），放进来是为了防止
// 后续改动把 resolve/sanitize 的越权校验改坏——它是 AI 文件工具的边界。
func TestSanitizeFileNameRejectsTraversal(t *testing.T) {
	rejected := []string{
		"..",
		"../",
		`..\`,
		"a/../b",
		`..\..\windows`,
		"  ..  ",
		"/etc/passwd",
		`C:\Windows\x`,
		"c:evil",
		`\\server\share`,
		"a\x00b",
	}
	for _, name := range rejected {
		if got, err := sanitizeFileName(name); err == nil {
			t.Errorf("sanitizeFileName(%q) = %q, err = nil; 期望被拒绝", name, got)
		}
	}
}

// TestSanitizeFileNameNormalizesHostileNames 确认清洗结果本身必须是安全的
// 纯文件名（不含路径分隔符、不以点或空格结尾）。
func TestSanitizeFileNameNormalizesHostileNames(t *testing.T) {
	cases := map[string]string{
		"a/b":       "b",
		"a//b":      "b",
		"foo.":      "foo",
		"foo...":    "foo",
		"a<b>c.md":  "a_b_c.md",
		"  x  .md ": "x .md",
	}
	for in, want := range cases {
		got, err := sanitizeFileName(in)
		if err != nil {
			t.Errorf("sanitizeFileName(%q) 返回错误: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("sanitizeFileName(%q) = %q, 期望 %q", in, got, want)
		}
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("sanitizeFileName(%q) = %q 仍含路径分隔符", in, got)
		}
	}
}

// TestDeriveFileNameStillWorks 防止审计期间的改动影响既有的文件名推导。
func TestDeriveFileNameStillWorks(t *testing.T) {
	cases := map[string]string{
		"# 水平运输系统\n内容":    "水平运输系统",
		"# \n\n## 水平运输系统": "水平运输系统",
		"**加粗标题**\n内容":    "加粗标题",
		"":                "未命名笔记",
	}
	for in, want := range cases {
		if got := deriveFileName(in); got != want {
			t.Errorf("deriveFileName(%q) = %q, 期望 %q", in, got, want)
		}
	}
}

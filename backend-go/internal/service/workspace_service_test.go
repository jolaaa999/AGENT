package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestWorkspace(t *testing.T) *WorkspaceService {
	t.Helper()
	return NewWorkspaceService(t.TempDir())
}

func TestWorkspaceWriteAndRead(t *testing.T) {
	w := newTestWorkspace(t)

	if err := w.WriteFile("alice", "notes/a.md", []byte("# hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := w.ReadFile("alice", "notes/a.md")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "# hello" {
		t.Fatalf("content mismatch: %q", got)
	}
}

// 核心安全断言：任何越权路径都必须被拒绝。
func TestWorkspaceRejectsEscape(t *testing.T) {
	w := newTestWorkspace(t)

	// 在用户目录外放一个「机密」文件
	outside := filepath.Join(filepath.Dir(w.Root()), "secret.txt")
	if err := os.WriteFile(outside, []byte("top secret"), 0o600); err != nil {
		t.Fatalf("setup outside file: %v", err)
	}
	defer os.Remove(outside)

	attacks := []string{
		"../secret.txt",
		"../../secret.txt",
		"notes/../../secret.txt",
		"..\\..\\secret.txt",
		"/etc/passwd",
		`C:\Windows\System32\drivers\etc\hosts`,
		"notes/../../../etc/passwd",
	}

	for _, attack := range attacks {
		if _, err := w.ReadFile("alice", attack); err == nil {
			t.Errorf("ReadFile(%q) unexpectedly succeeded (escape!)", attack)
		}
		if err := w.WriteFile("alice", attack, []byte("pwned")); err == nil {
			t.Errorf("WriteFile(%q) unexpectedly succeeded (escape!)", attack)
		}
	}

	// 确认越权写入没有真的发生
	if data, err := os.ReadFile(outside); err == nil && strings.Contains(string(data), "pwned") {
		t.Fatal("overwrote a file outside the workspace")
	}
}

// user_id 也必须是白名单，避免用它逃出根目录。
func TestWorkspaceRejectsBadUserID(t *testing.T) {
	w := newTestWorkspace(t)

	badIDs := []string{"../evil", "..", "a/b", `a\b`, "", "中文用户"}
	for _, id := range badIDs {
		if _, err := w.UserDir(id); err == nil {
			t.Errorf("UserDir(%q) unexpectedly succeeded", id)
		}
	}
}

func TestWorkspaceListSearchRenameDelete(t *testing.T) {
	w := newTestWorkspace(t)

	mustWrite := func(p, c string) {
		t.Helper()
		if err := w.WriteFile("bob", p, []byte(c)); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	mustWrite("a.md", "二叉树 前序遍历")
	mustWrite("sub/b.md", "二叉树 中序遍历")

	entries, err := w.ListFiles("bob")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %+v", len(entries), entries)
	}

	hits, err := w.SearchInFiles("bob", "二叉树", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d: %+v", len(hits), hits)
	}

	newRel, err := w.RenameFile("bob", "a.md", "renamed.md")
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if newRel != "renamed.md" {
		t.Fatalf("unexpected new path %q", newRel)
	}
	// 重命名不允许借机越权
	if _, err := w.RenameFile("bob", "renamed.md", "../escaped.md"); err == nil {
		t.Error("rename with ../ unexpectedly succeeded")
	}

	if err := w.DeleteFile("bob", "renamed.md"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	entries, _ = w.ListFiles("bob")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after delete, got %d", len(entries))
	}
}

func TestWorkspaceIsolationBetweenUsers(t *testing.T) {
	w := newTestWorkspace(t)

	if err := w.WriteFile("alice", "a.md", []byte("alice data")); err != nil {
		t.Fatalf("write: %v", err)
	}
	// bob 不应看到 alice 的文件
	entries, err := w.ListFiles("bob")
	if err != nil {
		t.Fatalf("list bob: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("bob sees %d files, expected 0 (isolation broken)", len(entries))
	}
	if _, err := w.ReadFile("bob", "a.md"); err == nil {
		t.Error("bob was able to read alice's file")
	}
}

func TestWorkspaceSaveOriginal(t *testing.T) {
	w := newTestWorkspace(t)

	path, err := w.SaveOriginal("carol", "讲义.pdf", []byte("%PDF-1.4 fake"))
	if err != nil {
		t.Fatalf("save original: %v", err)
	}
	if !strings.HasSuffix(filepath.ToSlash(path), "original/讲义.pdf") {
		t.Fatalf("unexpected original path: %s", path)
	}
	// 文件名里的路径穿越必须被拒绝（而不是悄悄清洗成另一个文件）
	if _, err := w.SaveOriginal("carol", "../../evil.pdf", []byte("x")); err == nil {
		t.Fatal("SaveOriginal with ../../ unexpectedly succeeded")
	}
	abs, _ := w.OriginalPath("carol", "讲义.pdf")
	if rel, err := filepath.Rel(w.Root(), abs); err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("original path escaped: %s", abs)
	}
}

// 上传文件名可能来自笔记标题，含换行/制表符/Windows 保留字符，
// 这些都会让写盘失败，必须被清洗成安全文件名。
func TestWorkspaceSanitizesHostileFileNames(t *testing.T) {
	w := newTestWorkspace(t)

	cases := []struct {
		input      string
		shouldWork bool
	}{
		{"KDD\r\n\r\n将 Latent Diffusion Mo", true},
		{"a\tb.md", true},
		{"report<2024>?.md", true},
		{"trailing.  ", true},
		{"normal.md", true},
		{"../escape.md", false},
		{"/abs/path.md", false},
	}

	for _, c := range cases {
		err := w.WriteFile("dave", c.input, []byte("content"))
		if c.shouldWork && err != nil {
			t.Errorf("WriteFile(%q) failed: %v", c.input, err)
		}
		if !c.shouldWork && err == nil {
			t.Errorf("WriteFile(%q) unexpectedly succeeded", c.input)
		}
	}

	// 确认写到磁盘上的名字里没有换行
	entries, err := w.ListFiles("dave")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, e := range entries {
		if strings.ContainsAny(e.Name, "\r\n\t") {
			t.Errorf("entry name still contains control chars: %q", e.Name)
		}
	}
	// 含换行的那个文件应被成功写入（清洗成空格）
	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name, "KDD") {
			found = true
		}
	}
	if !found {
		t.Error("expected the KDD file to be written under a sanitized name")
	}
}

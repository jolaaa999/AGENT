package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// WorkspaceService 管理「每个用户的受限工作区」。
//
// 设计目标：让 AI 学习导师能够对一个真实目录做文件操作（读取、写入、列出、
// 搜索、重命名），并把结果作为可下载产物交给用户；同时保证 AI 只能触碰
// 该用户自己的工作区，不能越出目录边界。
//
// 隔离策略（受限工作区，非容器）：
//   - 每个用户一个根目录 <root>/<user_id>/
//   - 所有路径都经过 resolve 做「清洗 + 前缀校验」，阻断 ../ 与绝对路径逃逸
//   - user_id 与文件名都做白名单化，避免路径分隔符与保留字符
type WorkspaceService struct {
	root string
}

// WorkspaceEntry 工作区中的一个文件条目。
type WorkspaceEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
	// IsOriginal 标记这是用户上传的原始文件（存于 original/ 子目录），
	// 与 AI 可编辑的 Markdown 正文区分开，便于前端分开呈现。
	IsOriginal bool `json:"is_original"`
	// Kind 是给前端用的展示类型：md / docx / pdf / image / other
	Kind string `json:"kind"`
}

var (
	// user_id 只允许字母、数字、下划线、连字符（中文等一律拒绝，避免目录名歧义）
	safeUserIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

func NewWorkspaceService(root string) *WorkspaceService {
	if strings.TrimSpace(root) == "" {
		root = filepath.Join(os.TempDir(), "learning-graph-workspaces")
	}
	return &WorkspaceService{root: root}
}

// Root 返回工作区根目录（便于启动时输出与排查）。
func (w *WorkspaceService) Root() string { return w.root }

// UserDir 返回（并按需创建）某用户的工作区目录。
func (w *WorkspaceService) UserDir(userID string) (string, error) {
	if err := validateUserID(userID); err != nil {
		return "", err
	}
	dir := filepath.Join(w.root, userID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create workspace dir: %w", err)
	}
	return dir, nil
}

func validateUserID(userID string) error {
	if !safeUserIDPattern.MatchString(userID) {
		return fmt.Errorf("invalid user id %q: only letters, digits, _ and - are allowed", userID)
	}
	return nil
}

// sanitizeFileName 清洗文件名，只保留基名并去掉路径分隔符与危险序列。
func sanitizeFileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("empty file name")
	}
	// 先做拒绝式校验：绝对路径与父子目录引用一律不接受。
	// 注意不能只靠 Base()——filepath.Base("/etc/passwd") 会「安全地」变成 passwd，
	// 这会让越权访问看起来合法，必须显式拒绝。
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return "", fmt.Errorf("absolute path is not allowed: %q", name)
	}
	// Windows 盘符（C:\...）与 UNC 路径
	if len(name) >= 2 && name[1] == ':' {
		return "", fmt.Errorf("absolute path is not allowed: %q", name)
	}
	if strings.HasPrefix(name, "//") {
		return "", fmt.Errorf("UNC path is not allowed: %q", name)
	}

	// 统一分隔符后取最后一段
	name = strings.ReplaceAll(name, "\\", "/")
	if name == ".." || strings.HasSuffix(name, "/..") || strings.Contains(name, "/../") {
		return "", fmt.Errorf("path traversal is not allowed: %q", name)
	}
	name = filepath.Base(name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("invalid file name %q", name)
	}
	if strings.ContainsAny(name, "\x00") {
		return "", errors.New("file name contains NUL byte")
	}

	// 去掉换行、回车、制表符等控制字符。
	// 上传时文件名可能来自笔记标题（如 "KDD\r\n\r\n将 Latent..."），
	// 这些字符会让 os.WriteFile 直接失败，且在各平台上行为不一致。
	name = strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1 // 其它控制字符直接丢弃
		}
		return r
	}, name)

	// 折叠连续空格，并去掉首尾空白
	name = strings.Join(strings.Fields(name), " ")
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("file name is empty after sanitizing")
	}

	// Windows 保留字符与结尾的点和空格都会导致写盘失败
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimRight(name, ". ")
	if name == "" {
		return "", errors.New("file name is empty after sanitizing")
	}

	// 限制长度，避免超长文件名导致写盘失败
	if len([]rune(name)) > 128 {
		r := []rune(name)
		name = string(r[:128])
	}
	return name, nil
}

// resolve 把工作区内的相对路径解析为绝对路径，并校验未逃出用户目录。
//
// 这是沙箱的关键防线：即使用户/AI 传入 "../../etc/passwd"，
// 也会被 Clean 归一化后由前缀校验拦下。
func (w *WorkspaceService) resolve(userID, relPath string) (string, error) {
	dir, err := w.UserDir(userID)
	if err != nil {
		return "", err
	}

	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return "", errors.New("empty path")
	}
	// 绝对路径直接拒绝，不做任何「截断成相对路径」的宽容处理
	if filepath.IsAbs(relPath) || strings.HasPrefix(relPath, "/") || strings.HasPrefix(relPath, "\\") {
		return "", fmt.Errorf("absolute path is not allowed: %q", relPath)
	}
	if len(relPath) >= 2 && relPath[1] == ':' {
		return "", fmt.Errorf("absolute path is not allowed: %q", relPath)
	}
	relPath = strings.ReplaceAll(relPath, "\\", "/")

	// 允许传入子目录（如 "notes/a.md"），逐段清洗
	parts := strings.Split(relPath, "/")
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." {
			continue
		}
		if p == ".." {
			return "", errors.New("path traversal is not allowed")
		}
		safe, err := sanitizeFileName(p)
		if err != nil {
			return "", err
		}
		cleaned = append(cleaned, safe)
	}
	if len(cleaned) == 0 {
		return "", errors.New("empty path")
	}

	abs := filepath.Join(append([]string{dir}, cleaned...)...)

	// 二次保险：解析后的绝对路径必须仍在用户目录之下
	rel, err := filepath.Rel(dir, abs)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", errors.New("path escapes workspace")
	}
	return abs, nil
}

// WriteFile 在工作区内写入文件（覆盖）。
func (w *WorkspaceService) WriteFile(userID, relPath string, content []byte) error {
	abs, err := w.resolve(userID, relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	return os.WriteFile(abs, content, 0o640)
}

// ReadFile 读取工作区内的文件。
func (w *WorkspaceService) ReadFile(userID, relPath string) ([]byte, error) {
	abs, err := w.resolve(userID, relPath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}

// ListFiles 列出工作区内的文件（递归，忽略子目录层级只回名字）。
func (w *WorkspaceService) ListFiles(userID string) ([]WorkspaceEntry, error) {
	dir, err := w.UserDir(userID)
	if err != nil {
		return nil, err
	}

	entries := make([]WorkspaceEntry, 0)
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 单个条目不可读时跳过，不影响整体列举
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			rel = d.Name()
		}
		relSlash := filepath.ToSlash(rel)
		entries = append(entries, WorkspaceEntry{
			Name:       relSlash,
			Size:       info.Size(),
			ModTime:    info.ModTime().UTC().Format(time.RFC3339),
			IsOriginal: strings.HasPrefix(relSlash, "original/"),
			Kind:       classifyFileKind(relSlash),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// DeleteFile 删除工作区内的文件。
func (w *WorkspaceService) DeleteFile(userID, relPath string) error {
	abs, err := w.resolve(userID, relPath)
	if err != nil {
		return err
	}
	return os.Remove(abs)
}

// RenameFile 在工作区内重命名文件（仅允许同目录内改名，避免移动越权）。
func (w *WorkspaceService) RenameFile(userID, oldPath, newName string) (string, error) {
	oldAbs, err := w.resolve(userID, oldPath)
	if err != nil {
		return "", err
	}
	// newName 必须是纯文件名：不接受任何带目录成分的输入，
	// 否则 "sub/x.md" 或 "../x.md" 会变成移动/越权。
	trimmed := strings.TrimSpace(strings.ReplaceAll(newName, "\\", "/"))
	if strings.Contains(trimmed, "/") {
		return "", fmt.Errorf("new name must be a plain file name, got %q", newName)
	}
	safeNew, err := sanitizeFileName(trimmed)
	if err != nil {
		return "", err
	}
	newAbs := filepath.Join(filepath.Dir(oldAbs), safeNew)

	// 目标同样要在工作区内
	dir, _ := w.UserDir(userID)
	if rel, err := filepath.Rel(dir, newAbs); err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("path escapes workspace")
	}
	if err := os.Rename(oldAbs, newAbs); err != nil {
		return "", err
	}
	outRel, _ := filepath.Rel(dir, newAbs)
	return filepath.ToSlash(outRel), nil
}

// SearchInFiles 在工作区内按关键字搜索，返回命中文件与所在行。
//
// 用途：让 AI 在「改文件」前能定位内容，而不需要把整份笔记读进上下文。
func (w *WorkspaceService) SearchInFiles(userID, keyword string, maxHits int) ([]map[string]interface{}, error) {
	if strings.TrimSpace(keyword) == "" {
		return nil, errors.New("empty keyword")
	}
	if maxHits <= 0 || maxHits > 200 {
		maxHits = 50
	}
	dir, err := w.UserDir(userID)
	if err != nil {
		return nil, err
	}

	hits := make([]map[string]interface{}, 0)
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || len(hits) >= maxHits {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, keyword) {
				hits = append(hits, map[string]interface{}{
					"file": filepath.ToSlash(rel),
					"line": i + 1,
					"text": strings.TrimSpace(line),
				})
				if len(hits) >= maxHits {
					return nil
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hits, nil
}

// ResolveForDownload 解析工作区内文件的绝对路径，供下载使用。
// 复用 resolve 的越权校验，保证下载接口也不能读取工作区之外的文件。
func (w *WorkspaceService) ResolveForDownload(userID, relPath string) (string, error) {
	abs, err := w.resolve(userID, relPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("文件不存在：%s", relPath)
	}
	if info.IsDir() {
		return "", errors.New("不能下载目录")
	}
	return abs, nil
}

// OriginalPath 返回用户上传原件在工作区中的存放路径（original/ 子目录）。
func (w *WorkspaceService) OriginalPath(userID, fileName string) (string, error) {
	safe, err := sanitizeFileName(fileName)
	if err != nil {
		return "", err
	}
	return w.resolve(userID, "original/"+safe)
}

// SaveOriginal 保存用户上传的原始文件（用于后续下载原件）。
func (w *WorkspaceService) SaveOriginal(userID, fileName string, raw []byte) (string, error) {
	abs, err := w.OriginalPath(userID, fileName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
		return "", fmt.Errorf("create original dir: %w", err)
	}
	if err := os.WriteFile(abs, raw, 0o640); err != nil {
		return "", err
	}
	return abs, nil
}

// classifyFileKind 按扩展名给出展示用的文件类型。
func classifyFileKind(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".txt":
		return "md"
	case ".docx", ".doc":
		return "word"
	case ".pdf":
		return "pdf"
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp":
		return "image"
	default:
		return "other"
	}
}

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"backend-go/internal/config"
	"backend-go/internal/model"
	"backend-go/internal/repository"
)

type GraphService interface {
	// 图谱核心
	UploadNote(ctx context.Context, req model.UploadNoteRequest, userID string) (map[string]interface{}, error)
	UploadNoteLangChain(ctx context.Context, req model.UploadNoteRequest, userID string) (map[string]interface{}, error)
	GetGraphAll(ctx context.Context, userID string) (model.G6GraphResponse, error)
	GetGraphByFile(ctx context.Context, userID, fileID string) (model.G6GraphResponse, error)
	GetGraphByFileGroup(ctx context.Context, userID, fileGroupID string) (model.G6GraphResponse, error)
	GetGraphPath(ctx context.Context, userID, concept string, maxDepth int) (model.PathResponse, error)
	ExplainConcept(ctx context.Context, req model.ExplainRequest, userID string) (model.ExplainResponse, error)
	GetNodeNeighbors(ctx context.Context, userID string, nodeID string, depth int) (model.G6GraphResponse, error)
	// AI 对话
	ChatWithContext(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error)
	LearningPath(ctx context.Context, req model.LearningPathRequest) (model.LearningPathResponse, error)
	// 文件转换（docx/pdf/图片 → Markdown）
	ExtractToMarkdown(ctx context.Context, filename string, raw []byte) (map[string]interface{}, error)
	// 用户工作区（AI 可操作的文件沙箱）
	Workspace() *WorkspaceService
	SyncFileToWorkspace(ctx context.Context, userID, fileID string) (string, error)
	// AI 文件工具（Python Agent 通过回调调用）
	WorkspaceList(userID string) ([]WorkspaceEntry, error)
	WorkspaceRead(userID, path string) (string, error)
	WorkspaceWrite(userID, path, content string) error
	WorkspaceSearch(userID, keyword string, maxHits int) ([]map[string]interface{}, error)
	WorkspaceRename(userID, path, newName string) (string, error)
	WorkspaceDelete(userID, path string) error
	// 文件管理
	CreateFile(ctx context.Context, userID, name, fileGroupID string) (string, error)
	CreateFileGroup(ctx context.Context, userID, name string) (string, error)
	ListUserFiles(ctx context.Context, userID string) ([]model.UserFile, []model.FileGroup, error)
	GetFilesMarkdown(ctx context.Context, userID, fileID, fileGroupID string) (string, error)
	UpdateFileContent(ctx context.Context, userID, fileID, content string) error
	DeleteFile(ctx context.Context, userID, fileID string) error
	DeleteFileGroup(ctx context.Context, userID, groupID string) error
	RenameFile(ctx context.Context, userID, fileID, newName string) error
	RenameFileGroup(ctx context.Context, userID, groupID, newName string) error
	AddFileToGroup(ctx context.Context, userID, fileID, groupID string) error
	TogglePinFile(ctx context.Context, userID, fileID string) error
	TogglePinFileGroup(ctx context.Context, userID, groupID string) error
	// 对话管理
	GetOrCreateConversation(ctx context.Context, userID, fileID, fileGroupID string) (model.Conversation, error)
	SaveMessage(ctx context.Context, req model.SaveMessageRequest, userID string) error
	GetConversation(ctx context.Context, userID, conversationID string) (model.Conversation, error)
	DeleteConversation(ctx context.Context, userID, conversationID string) error
}

type graphService struct {
	cfg        config.Config
	repository repository.GraphRepository
	client     *http.Client
	workspace  *WorkspaceService
}

func NewGraphService(cfg config.Config, repo repository.GraphRepository) GraphService {
	return &graphService{
		cfg:        cfg,
		repository: repo,
		client: &http.Client{
			Timeout: time.Duration(cfg.PythonTimeoutSec) * time.Second,
		},
		workspace: NewWorkspaceService(cfg.WorkspaceRoot),
	}
}

// Workspace 暴露工作区实例，供控制器做文件下载等直连操作。
func (s *graphService) Workspace() *WorkspaceService { return s.workspace }

// ==================== 图谱上传（原始 DeepSeek SDK） ====================

func (s *graphService) UploadNote(ctx context.Context, req model.UploadNoteRequest, userID string) (map[string]interface{}, error) {
	parseReq := model.ParseRequest{Markdown: req.Markdown}
	payload, err := json.Marshal(parseReq)
	if err != nil {
		return nil, fmt.Errorf("marshal parse request: %w", err)
	}

	body, err := s.callPython(ctx, "POST", "/api/parse", payload)
	if err != nil {
		return nil, err
	}

	var parseResp model.ParseResponse
	if err := json.Unmarshal(body, &parseResp); err != nil {
		return nil, fmt.Errorf("unmarshal parse response: %w", err)
	}

	graphData := normalizeGraphData(parseResp, req.FileID, req.FileGroupID)
	if err := s.repository.UpsertGraph(ctx, userID, graphData); err != nil {
		return nil, fmt.Errorf("persist graph data into neo4j: %w", err)
	}

	return map[string]interface{}{
		"user_id":          userID,
		"chunks_count":     len(parseResp.Chunks),
		"entities_count":   len(graphData.Entities),
		"relations_count":  len(graphData.Relations),
		"llm_retries_used": parseResp.RetriesUse,
		"parser_result":    parseResp,
	}, nil
}

// ==================== 图谱上传（LangChain 诊断流水线） ====================

// deriveFileName 从笔记内容推导一个可读的文件名。
//
// 取首个非空行（通常是 Markdown 标题）并去掉标题符号，按 Unicode 字符截断到
// 30 个字符；若没有可用标题则回退到首个非空内容行。
// 使用 rune 切片而非字节切片，确保中文文件名不会被截成乱码。
func deriveFileName(markdown string) string {
	const maxRunes = 30

	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		// 去掉 Markdown 标题符号与强调符号，让文件名更干净
		trimmed = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		trimmed = strings.NewReplacer("**", "", "__", "", "`", "").Replace(trimmed)
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == "" {
			continue
		}
		runes := []rune(trimmed)
		if len(runes) > maxRunes {
			return string(runes[:maxRunes]) + "…"
		}
		return string(runes)
	}

	// 内容为空或全是空白行时的兜底，避免产生空文件名。
	//
	// 注意：这里必须同样剥掉 Markdown 标记符号。
	// 否则像「#\n\n正文…」这种首行是空标题的笔记，兜底取到的会是 "#"，
	// 补上 .md 后缀后文件名就成了 ".md"（隐藏文件），用户根本看不出是什么。
	name := ""
	for _, line := range strings.Split(markdown, "\n") {
		cleaned := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
		cleaned = strings.NewReplacer("**", "", "__", "", "`", "").Replace(cleaned)
		if cleaned = strings.TrimSpace(cleaned); cleaned != "" {
			name = cleaned
			break
		}
	}
	runes := []rune(name)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	name = strings.TrimSpace(string(runes))
	if name == "" {
		return "未命名笔记"
	}
	// 兜底清理：保证是合法 UTF-8
	if !utf8.ValidString(name) {
		name = strings.ToValidUTF8(name, "")
	}
	return name
}

func (s *graphService) UploadNoteLangChain(ctx context.Context, req model.UploadNoteRequest, userID string) (map[string]interface{}, error) {
	// 保存正文，供文件组对话聚合、工作区同步与「下载改后稿」使用。
	//
	// 关键：无论新建还是往「已有文件」里追加内容，都必须落库。
	// 原实现只在 fileID 为空时保存，而前端上传时总会带上 file_id，
	// 于是正文永远写不进去——这正是旧文件 content 全部为空的原因。
	fileID := req.FileID
	if fileID == "" {
		var err error
		fileID, err = s.repository.CreateFileWithContent(
			ctx, userID, deriveFileName(req.Markdown), req.FileGroupID, req.Markdown,
		)
		if err != nil {
			return nil, fmt.Errorf("auto-create file: %w", err)
		}
	} else if strings.TrimSpace(req.Markdown) != "" {
		if err := s.repository.UpdateFileContent(ctx, userID, fileID, req.Markdown); err != nil {
			return nil, fmt.Errorf("save markdown for existing file %s: %w", fileID, err)
		}
	}

	diagReq := model.LangChainDiagnoseRequest{Markdown: req.Markdown}
	payload, err := json.Marshal(diagReq)
	if err != nil {
		return nil, fmt.Errorf("marshal langchain diagnose request: %w", err)
	}

	body, err := s.callPython(ctx, "POST", "/api/langchain/diagnose", payload)
	if err != nil {
		return nil, err
	}

	var diagResp model.LangChainDiagnoseResponse
	if err := json.Unmarshal(body, &diagResp); err != nil {
		return nil, fmt.Errorf("unmarshal langchain diagnose response: %w", err)
	}
	if !diagResp.Success {
		return nil, fmt.Errorf("langchain diagnose failed: %s", diagResp.ErrorMessage)
	}

	// 将 LangChain 响应转换为 GraphData
	graphData := normalizeLangChainData(diagResp, fileID, req.FileGroupID)
	if err := s.repository.UpsertGraph(ctx, userID, graphData); err != nil {
		return nil, fmt.Errorf("persist langchain graph data into neo4j: %w", err)
	}

	// 统计各状态数量
	errorCount := 0
	supplementCount := 0
	for _, entity := range graphData.Entities {
		switch entity.Status {
		case "error":
			errorCount++
		case "supplement":
			supplementCount++
		}
	}

	return map[string]interface{}{
		"user_id":          userID,
		"file_id":          fileID,
		"entities_count":   len(graphData.Entities),
		"relations_count":  len(graphData.Relations),
		"error_count":      errorCount,
		"supplement_count": supplementCount,
		"summary":          diagResp.Summary,
		"retries_used":     diagResp.RetriesUsed,
		"diagnose_result":  diagResp,
	}, nil
}

// ==================== 图谱查询 ====================

func (s *graphService) GetGraphAll(ctx context.Context, userID string) (model.G6GraphResponse, error) {
	return s.repository.GetGraphAll(ctx, userID)
}

func (s *graphService) GetGraphByFile(ctx context.Context, userID, fileID string) (model.G6GraphResponse, error) {
	return s.repository.GetGraphByFile(ctx, userID, fileID)
}

func (s *graphService) GetGraphByFileGroup(ctx context.Context, userID, fileGroupID string) (model.G6GraphResponse, error) {
	return s.repository.GetGraphByFileGroup(ctx, userID, fileGroupID)
}

func (s *graphService) GetGraphPath(ctx context.Context, userID, concept string, maxDepth int) (model.PathResponse, error) {
	return s.repository.GetPathsToConcept(ctx, userID, concept, maxDepth)
}

func (s *graphService) GetNodeNeighbors(ctx context.Context, userID string, nodeID string, depth int) (model.G6GraphResponse, error) {
	return s.repository.GetNodeNeighbors(ctx, userID, nodeID, depth)
}

// ==================== 概念讲解 ====================

func (s *graphService) ExplainConcept(ctx context.Context, req model.ExplainRequest, userID string) (model.ExplainResponse, error) {
	payload, err := json.Marshal(map[string]string{
		"concept":  strings.TrimSpace(req.Concept),
		"markdown": req.Markdown,
	})
	if err != nil {
		return model.ExplainResponse{}, fmt.Errorf("marshal explain request: %w", err)
	}

	body, err := s.callPython(ctx, "POST", "/api/explain", payload)
	if err != nil {
		return model.ExplainResponse{}, err
	}

	var explainResp model.ExplainResponse
	if err := json.Unmarshal(body, &explainResp); err != nil {
		return model.ExplainResponse{}, fmt.Errorf("unmarshal explain response: %w", err)
	}
	return explainResp, nil
}

// ==================== AI 对话 ====================

func (s *graphService) ChatWithContext(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return model.ChatResponse{}, fmt.Errorf("marshal chat request: %w", err)
	}

	body, err := s.callPython(ctx, "POST", "/api/langchain/chat", payload)
	if err != nil {
		return model.ChatResponse{}, err
	}

	var chatResp model.ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return model.ChatResponse{}, fmt.Errorf("unmarshal chat response: %w", err)
	}
	return chatResp, nil
}

func (s *graphService) LearningPath(ctx context.Context, req model.LearningPathRequest) (model.LearningPathResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return model.LearningPathResponse{}, fmt.Errorf("marshal learning path request: %w", err)
	}

	body, err := s.callPython(ctx, "POST", "/api/langchain/learning-path", payload)
	if err != nil {
		return model.LearningPathResponse{}, err
	}

	var pathResp model.LearningPathResponse
	if err := json.Unmarshal(body, &pathResp); err != nil {
		return model.LearningPathResponse{}, fmt.Errorf("unmarshal learning path response: %w", err)
	}
	return pathResp, nil
}

// ==================== 用户工作区（AI 文件沙箱） ====================

// SyncFileToWorkspace 把某个已入库文件的正文写入用户工作区，返回工作区相对路径。
//
// 这样 AI 就能像操作真实文件一样读取/修改它，用户也能把修改结果下载走。
// 文件名取入库时的 name（已由 deriveFileName 清洗过），必要时降级为 file_id。
func (s *graphService) SyncFileToWorkspace(ctx context.Context, userID, fileID string) (string, error) {
	data, err := s.repository.GetFilesMarkdown(ctx, userID, fileID, "")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(data) == "" {
		// 兜底：早期上传的文件没有保存正文（File.content 为空），
		// 但概念节点仍在图谱里。用图谱重建一份可用文档，
		// 让老文件也能进入工作区被 AI 读取和修改，而不是直接失败。
		rebuilt, rerr := s.repository.RebuildMarkdownFromGraph(ctx, userID, fileID)
		if rerr != nil {
			return "", fmt.Errorf("rebuild markdown from graph for %s: %w", fileID, rerr)
		}
		if strings.TrimSpace(rebuilt) == "" {
			return "", fmt.Errorf("file %s has no content to sync", fileID)
		}
		data = rebuilt
	}

	// 取可读文件名；失败则退回 file_id.md
	name := fileID + ".md"
	if files, _, lerr := s.repository.ListUserFiles(ctx, userID); lerr == nil {
		for _, f := range files {
			if f.ID == fileID {
				if strings.TrimSpace(f.Name) != "" {
					name = ensureMarkdownExt(f.Name)
				}
				break
			}
		}
	}

	if err := s.workspace.WriteFile(userID, name, []byte(data)); err != nil {
		return "", err
	}
	return name, nil
}

// ensureMarkdownExt 保证文件名以 .md 结尾（入库名可能是标题文本）。
func ensureMarkdownExt(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "note.md"
	}
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
		return name
	}
	return name + ".md"
}

func (s *graphService) WorkspaceList(userID string) ([]WorkspaceEntry, error) {
	return s.workspace.ListFiles(userID)
}

func (s *graphService) WorkspaceRead(userID, path string) (string, error) {
	data, err := s.workspace.ReadFile(userID, path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *graphService) WorkspaceWrite(userID, path, content string) error {
	return s.workspace.WriteFile(userID, path, []byte(content))
}

func (s *graphService) WorkspaceSearch(userID, keyword string, maxHits int) ([]map[string]interface{}, error) {
	return s.workspace.SearchInFiles(userID, keyword, maxHits)
}

func (s *graphService) WorkspaceRename(userID, path, newName string) (string, error) {
	return s.workspace.RenameFile(userID, path, newName)
}

func (s *graphService) WorkspaceDelete(userID, path string) error {
	return s.workspace.DeleteFile(userID, path)
}

// ==================== 文件转换 ====================

// ExtractToMarkdown 把上传的 docx / pdf / 图片 等文件转成 Markdown。
//
// 实际解析在 Python 侧完成（那里有 python-docx / pypdf / 多模态识图能力），
// Go 只负责以 multipart 形式转发文件流，保持 Frontend→Go→Python 的分层。
func (s *graphService) ExtractToMarkdown(ctx context.Context, filename string, raw []byte) (map[string]interface{}, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("build multipart form: %w", err)
	}
	if _, err := part.Write(raw); err != nil {
		return nil, fmt.Errorf("write file into multipart form: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	url := strings.TrimRight(s.cfg.PythonServiceURL, "/") + "/api/extract/to-markdown"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, &buf)
	if err != nil {
		return nil, fmt.Errorf("create extract request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	// 转换（尤其图片识图）比普通请求慢，使用更宽松的超时
	client := &http.Client{Timeout: time.Duration(s.cfg.PythonTimeoutSec) * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request python extract service: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read extract response: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		// 透传 Python 侧的可读原因（如「不支持 .doc 格式」）
		var detail struct {
			Detail string `json:"detail"`
		}
		if json.Unmarshal(body, &detail) == nil && detail.Detail != "" {
			return nil, fmt.Errorf("%s", detail.Detail)
		}
		return nil, fmt.Errorf("extract service error (%d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("unmarshal extract response: %w", err)
	}
	return result, nil
}

// ==================== 文件管理 ====================

func (s *graphService) CreateFile(ctx context.Context, userID, name, fileGroupID string) (string, error) {
	return s.repository.CreateFile(ctx, userID, name, fileGroupID)
}

func (s *graphService) CreateFileGroup(ctx context.Context, userID, name string) (string, error) {
	return s.repository.CreateFileGroup(ctx, userID, name)
}

func (s *graphService) ListUserFiles(ctx context.Context, userID string) ([]model.UserFile, []model.FileGroup, error) {
	return s.repository.ListUserFiles(ctx, userID)
}

// GetFilesMarkdown 读取指定文件（或整个文件组）的正文，供前端作为对话上下文。
func (s *graphService) GetFilesMarkdown(ctx context.Context, userID, fileID, fileGroupID string) (string, error) {
	return s.repository.GetFilesMarkdown(ctx, userID, fileID, fileGroupID)
}

// UpdateFileContent 保存 AI 编辑后的正文到指定文件。
func (s *graphService) UpdateFileContent(ctx context.Context, userID, fileID, content string) error {
	if strings.TrimSpace(fileID) == "" {
		return fmt.Errorf("file_id is required")
	}
	return s.repository.UpdateFileContent(ctx, userID, fileID, content)
}

func (s *graphService) DeleteFile(ctx context.Context, userID, fileID string) error {
	return s.repository.DeleteFile(ctx, userID, fileID)
}

func (s *graphService) DeleteFileGroup(ctx context.Context, userID, groupID string) error {
	return s.repository.DeleteFileGroup(ctx, userID, groupID)
}

func (s *graphService) RenameFile(ctx context.Context, userID, fileID, newName string) error {
	return s.repository.RenameFile(ctx, userID, fileID, newName)
}

func (s *graphService) RenameFileGroup(ctx context.Context, userID, groupID, newName string) error {
	return s.repository.RenameFileGroup(ctx, userID, groupID, newName)
}

func (s *graphService) AddFileToGroup(ctx context.Context, userID, fileID, groupID string) error {
	return s.repository.AddFileToGroup(ctx, userID, fileID, groupID)
}

func (s *graphService) TogglePinFile(ctx context.Context, userID, fileID string) error {
	return s.repository.TogglePinFile(ctx, userID, fileID)
}

func (s *graphService) TogglePinFileGroup(ctx context.Context, userID, groupID string) error {
	return s.repository.TogglePinFileGroup(ctx, userID, groupID)
}

// ==================== 对话管理 ====================

func (s *graphService) GetOrCreateConversation(ctx context.Context, userID, fileID, fileGroupID string) (model.Conversation, error) {
	return s.repository.GetOrCreateConversation(ctx, userID, fileID, fileGroupID)
}

func (s *graphService) SaveMessage(ctx context.Context, req model.SaveMessageRequest, userID string) error {
	return s.repository.SaveMessage(ctx, req, userID)
}

func (s *graphService) GetConversation(ctx context.Context, userID, conversationID string) (model.Conversation, error) {
	return s.repository.GetConversation(ctx, userID, conversationID)
}

func (s *graphService) DeleteConversation(ctx context.Context, userID, conversationID string) error {
	return s.repository.DeleteConversation(ctx, userID, conversationID)
}

// ==================== 内部辅助 ====================

func (s *graphService) callPython(ctx context.Context, method, path string, payload []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(
		ctx, method,
		strings.TrimRight(s.cfg.PythonServiceURL, "/")+path,
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("create python request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request python service %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("python service error (%d) at %s: %s", resp.StatusCode, path, string(body))
	}
	return body, nil
}

// normalizeGraphData 将旧版 ParseResponse 转换为 GraphData
func normalizeGraphData(parseResp model.ParseResponse, fileID, fileGroupID string) model.GraphData {
	entityMap := map[string]model.Entity{}
	relations := make([]model.Relation, 0, len(parseResp.Relations))

	for _, rel := range parseResp.Relations {
		source := strings.TrimSpace(rel.Source)
		target := strings.TrimSpace(rel.Target)
		if source == "" || target == "" {
			continue
		}

		if _, exists := entityMap[source]; !exists {
			entityMap[source] = model.Entity{
				Name: source, Type: "Concept",
				FileID: fileID, FileGroupID: fileGroupID,
				Properties: map[string]interface{}{},
			}
		}
		if _, exists := entityMap[target]; !exists {
			entityMap[target] = model.Entity{
				Name: target, Type: "Concept",
				Status: rel.Status, Reason: rel.Reason,
				FileID: fileID, FileGroupID: fileGroupID,
				Properties: map[string]interface{}{},
			}
		}

		relations = append(relations, model.Relation{
			Source: source, Target: target,
			Type: rel.Relation, Description: rel.Relation,
			Status: rel.Status, Reason: rel.Reason,
			Properties: map[string]interface{}{"relation": rel.Relation},
		})
	}

	entities := make([]model.Entity, 0, len(entityMap))
	for _, entity := range entityMap {
		entities = append(entities, entity)
	}
	return model.GraphData{Entities: entities, Relations: relations}
}

// normalizeLangChainData 将 LangChain 诊断结果转换为 GraphData
func normalizeLangChainData(diag model.LangChainDiagnoseResponse, fileID, fileGroupID string) model.GraphData {
	entityMap := map[string]model.Entity{}
	relations := make([]model.Relation, 0, len(diag.Edges))

	for _, node := range diag.Nodes {
		name := strings.TrimSpace(node.Name)
		if name == "" {
			continue
		}
		entityMap[name] = model.Entity{
			Name: name, Type: fallbackS(node.EntityType, "Concept"),
			Status: node.Status, Reason: node.Reason,
			FileID: fileID, FileGroupID: fileGroupID,
			Properties: map[string]interface{}{
				"definition": node.Definition,
				"source":     node.Source,
			},
		}
	}

	for _, edge := range diag.Edges {
		source := strings.TrimSpace(edge.Source)
		target := strings.TrimSpace(edge.Target)
		if source == "" || target == "" {
			continue
		}
		// 确保源和目标实体存在
		if _, exists := entityMap[source]; !exists {
			entityMap[source] = model.Entity{
				Name: source, Type: "Concept",
				FileID: fileID, FileGroupID: fileGroupID,
				Properties: map[string]interface{}{},
			}
		}
		if _, exists := entityMap[target]; !exists {
			entityMap[target] = model.Entity{
				Name: target, Type: "Concept",
				FileID: fileID, FileGroupID: fileGroupID,
				Properties: map[string]interface{}{},
			}
		}

		relations = append(relations, model.Relation{
			Source: source, Target: target,
			Type:        fallbackS(edge.Relation, "RELATED_TO"),
			Description: edge.Relation,
			Status:      edge.Status, Reason: edge.Reason,
			Properties: map[string]interface{}{
				"relation":     edge.Relation,
				"source_agent": edge.SourceAgent,
			},
		})
	}

	entities := make([]model.Entity, 0, len(entityMap))
	for _, entity := range entityMap {
		entities = append(entities, entity)
	}
	return model.GraphData{Entities: entities, Relations: relations}
}

func fallbackS(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

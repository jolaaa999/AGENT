package controller

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"backend-go/internal/model"
	"backend-go/internal/service"

	"github.com/gin-gonic/gin"
)

type GraphController struct {
	service       service.GraphService
	defaultUserID string
}

func NewGraphController(graphService service.GraphService, defaultUserID string) *GraphController {
	return &GraphController{service: graphService, defaultUserID: defaultUserID}
}

// ==================== 图谱上传 ====================

func (gc *GraphController) UploadNote(c *gin.Context) {
	var req model.UploadNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, req.UserID)

	var result map[string]interface{}
	var err error

	if req.UseLangChain {
		result, err = gc.service.UploadNoteLangChain(c.Request.Context(), req, userID)
	} else {
		result, err = gc.service.UploadNote(c.Request.Context(), req, userID)
	}

	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to upload note", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// UploadNoteLangChain 独立 LangChain 端点（前端可直接调用）
func (gc *GraphController) UploadNoteLangChain(c *gin.Context) {
	var req model.UploadNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	req.UseLangChain = true
	userID := gc.resolveUserID(c, req.UserID)
	result, err := gc.service.UploadNoteLangChain(c.Request.Context(), req, userID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "langchain diagnose failed", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ==================== 图谱查询 ====================

func (gc *GraphController) GetGraphAll(c *gin.Context) {
	userID := gc.resolveUserID(c, "")

	// 支持按 file_id 或 file_group_id 筛选
	if fileID := strings.TrimSpace(c.Query("file_id")); fileID != "" {
		graph, err := gc.service.GetGraphByFile(c.Request.Context(), userID, fileID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch graph by file", "detail": err.Error()})
			return
		}
		c.JSON(http.StatusOK, graph)
		return
	}
	if groupID := strings.TrimSpace(c.Query("file_group_id")); groupID != "" {
		graph, err := gc.service.GetGraphByFileGroup(c.Request.Context(), userID, groupID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch graph by file group", "detail": err.Error()})
			return
		}
		c.JSON(http.StatusOK, graph)
		return
	}

	graph, err := gc.service.GetGraphAll(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch graph", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, graph)
}

// ==================== 路径导航 ====================

func (gc *GraphController) GetGraphPath(c *gin.Context) {
	concept := strings.TrimSpace(c.Query("concept"))
	if concept == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "concept query param is required"})
		return
	}
	maxDepth := 3
	if value := c.Query("maxDepth"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "maxDepth must be an integer"})
			return
		}
		maxDepth = parsed
	}
	userID := gc.resolveUserID(c, "")
	paths, err := gc.service.GetGraphPath(c.Request.Context(), userID, concept, maxDepth)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch concept paths", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, paths)
}

// ==================== 概念讲解 ====================

func (gc *GraphController) ExplainConcept(c *gin.Context) {
	var req model.ExplainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, req.UserID)
	result, err := gc.service.ExplainConcept(c.Request.Context(), req, userID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to explain concept", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ==================== 邻居查询 ====================

func (gc *GraphController) GetNodeNeighbors(c *gin.Context) {
	nodeID := strings.TrimSpace(c.Query("node_id"))
	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id query param is required"})
		return
	}
	depth := 1
	if value := c.Query("depth"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "depth must be an integer"})
			return
		}
		depth = parsed
	}
	userID := gc.resolveUserID(c, "")
	result, err := gc.service.GetNodeNeighbors(c.Request.Context(), userID, nodeID, depth)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch node neighbors", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ==================== AI 对话 ====================

func (gc *GraphController) ChatWithContext(c *gin.Context) {
	var req model.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	// 用户身份优先取请求体（前端按登录态填写），其次 header/query，最后默认用户。
	// 注意：不能传空串调用 resolveUserID，否则会忽略 body 里的 user_id，
	// 导致 AI 工作区被错误地解析成 default_user。
	req.UserID = gc.resolveUserID(c, req.UserID)
	result, err := gc.service.ChatWithContext(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "chat failed", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (gc *GraphController) LearningPath(c *gin.Context) {
	var req model.LearningPathRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	result, err := gc.service.LearningPath(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "learning path generation failed", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// ==================== 文件管理 ====================

func (gc *GraphController) ListUserFiles(c *gin.Context) {
	userID := gc.resolveUserID(c, "")
	files, groups, err := gc.service.ListUserFiles(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list files", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"files": files, "file_groups": groups})
}

// GetFilesMarkdown 返回指定文件或整个文件组的 Markdown 正文。
//
// 供前端在「文件组对话」时聚合组内全部笔记作为上下文；
// 只传 file_group_id 即为整组内容，只传 file_id 则为单个文件。
func (gc *GraphController) GetFilesMarkdown(c *gin.Context) {
	userID := gc.resolveUserID(c, "")
	fileID := strings.TrimSpace(c.Query("file_id"))
	fileGroupID := strings.TrimSpace(c.Query("file_group_id"))
	if fileID == "" && fileGroupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_id or file_group_id is required"})
		return
	}
	markdown, err := gc.service.GetFilesMarkdown(c.Request.Context(), userID, fileID, fileGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get files markdown", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"markdown": markdown, "length": len(markdown)})
}

// ExtractToMarkdown 接收上传文件（docx/pdf/图片等），返回转好的 Markdown。
//
// 前端拿到 markdown 后再走既有的「生成图谱」流程，避免改动图谱写入链路。
func (gc *GraphController) ExtractToMarkdown(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未收到上传文件", "detail": err.Error()})
		return
	}
	const maxBytes = 20 << 20 // 20MB
	if fileHeader.Size > maxBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "文件过大（>20MB），请拆分后上传"})
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无法打开上传文件", "detail": err.Error()})
		return
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取上传文件失败", "detail": err.Error()})
		return
	}

	result, err := gc.service.ExtractToMarkdown(c.Request.Context(), fileHeader.Filename, raw)
	if err != nil {
		// 转换失败通常是「格式不支持/扫描件无文字」这类用户可理解的原因
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 保留原件，便于用户之后下载原始文件（docx/pdf/图片）
	//
	// 注意：必须显式读取 multipart 表单里的 user_id。
	// 原实现传的是空字符串，resolveUserID 于是回退到 default_user，
	// 导致所有用户的原件都堆在同一个目录、谁也下载不到自己的原件。
	userID := gc.resolveUserID(c, c.PostForm("user_id"))
	if saved, serr := gc.service.Workspace().SaveOriginal(userID, fileHeader.Filename, raw); serr == nil {
		result["original_name"] = filepath.Base(saved)
	} else {
		// 原件保存失败不应阻断转换结果，仅记录提示
		result["original_save_warning"] = serr.Error()
	}

	c.JSON(http.StatusOK, result)
}

// UpdateFileContent 保存 AI 编辑后的正文到指定文件（使其在刷新后仍保留）。
func (gc *GraphController) UpdateFileContent(c *gin.Context) {
	var req struct {
		FileID  string `json:"file_id"`
		Content string `json:"content"`
		UserID  string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, req.UserID)
	if err := gc.service.UpdateFileContent(c.Request.Context(), userID, req.FileID, req.Content); err != nil {
		respondResourceError(c, "failed to update file content", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}

// ==================== 用户工作区（AI 文件沙箱） ====================

// WorkspaceList 列出当前用户工作区内的文件。
func (gc *GraphController) WorkspaceList(c *gin.Context) {
	userID := gc.resolveUserID(c, "")
	entries, err := gc.service.WorkspaceList(userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_id": userID, "files": entries})
}

// WorkspaceRead 读取工作区内某个文件。
func (gc *GraphController) WorkspaceRead(c *gin.Context) {
	userID := gc.resolveUserID(c, "")
	path := strings.TrimSpace(c.Query("path"))
	content, err := gc.service.WorkspaceRead(userID, path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"path": path, "content": content, "length": len(content)})
}

// WorkspaceWrite 覆盖写入工作区内的文件。
func (gc *GraphController) WorkspaceWrite(c *gin.Context) {
	var req struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		UserID  string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, req.UserID)
	if err := gc.service.WorkspaceWrite(userID, req.Path, req.Content); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "written", "path": req.Path})
}

// WorkspaceSearch 在工作区内按关键字搜索。
func (gc *GraphController) WorkspaceSearch(c *gin.Context) {
	userID := gc.resolveUserID(c, "")
	keyword := strings.TrimSpace(c.Query("keyword"))
	maxHits, _ := strconv.Atoi(c.DefaultQuery("max_hits", "50"))
	hits, err := gc.service.WorkspaceSearch(userID, keyword, maxHits)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"keyword": keyword, "hits": hits, "count": len(hits)})
}

// WorkspaceRename 重命名工作区内的文件。
func (gc *GraphController) WorkspaceRename(c *gin.Context) {
	var req struct {
		Path    string `json:"path"`
		NewName string `json:"new_name"`
		UserID  string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, req.UserID)
	newPath, err := gc.service.WorkspaceRename(userID, req.Path, req.NewName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "renamed", "path": newPath})
}

// WorkspaceDelete 删除工作区内的文件。
func (gc *GraphController) WorkspaceDelete(c *gin.Context) {
	userID := gc.resolveUserID(c, "")
	path := strings.TrimSpace(c.Query("path"))
	if err := gc.service.WorkspaceDelete(userID, path); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted", "path": path})
}

// SyncFileToWorkspace 把某个已入库文件同步进工作区（AI 操作前准备）。
func (gc *GraphController) SyncFileToWorkspace(c *gin.Context) {
	var req struct {
		FileID string `json:"file_id"`
		UserID string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, req.UserID)
	path, err := gc.service.SyncFileToWorkspace(c.Request.Context(), userID, req.FileID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "synced", "path": path})
}

// DownloadWorkspaceFile 下载工作区内的文件（AI 修改后的产物）。
func (gc *GraphController) DownloadWorkspaceFile(c *gin.Context) {
	userID := gc.resolveUserID(c, "")
	path := strings.TrimSpace(c.Query("path"))
	abs, err := gc.service.Workspace().ResolveForDownload(userID, path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 用 Clean 后的基名做下载名，避免把用户输入的路径原样塞进响应头
	c.FileAttachment(abs, filepath.Base(abs))
}

// DownloadOriginalFile 下载用户最初上传的原件（docx/pdf/图片等）。
func (gc *GraphController) DownloadOriginalFile(c *gin.Context) {
	userID := gc.resolveUserID(c, "")
	name := strings.TrimSpace(c.Query("name"))
	abs, err := gc.service.Workspace().OriginalPath(userID, name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, statErr := os.Stat(abs); statErr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "未找到该原件，可能未以文件方式上传"})
		return
	}
	c.FileAttachment(abs, filepath.Base(abs))
}

func (gc *GraphController) CreateFile(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		FileGroupID string `json:"file_group_id"`
		UserID      string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, req.UserID)
	fileID, err := gc.service.CreateFile(c.Request.Context(), userID, req.Name, req.FileGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create file", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"file_id": fileID, "name": req.Name})
}

func (gc *GraphController) CreateFileGroup(c *gin.Context) {
	var req struct {
		Name   string `json:"name" binding:"required"`
		UserID string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, req.UserID)
	groupID, err := gc.service.CreateFileGroup(c.Request.Context(), userID, req.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create file group", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"group_id": groupID, "name": req.Name})
}

func (gc *GraphController) DeleteFile(c *gin.Context) {
	fileID := strings.TrimSpace(c.Query("file_id"))
	if fileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_id query param is required"})
		return
	}
	userID := gc.resolveUserID(c, "")
	if err := gc.service.DeleteFile(c.Request.Context(), userID, fileID); err != nil {
		respondResourceError(c, "failed to delete file", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (gc *GraphController) DeleteFileGroup(c *gin.Context) {
	groupID := strings.TrimSpace(c.Query("group_id"))
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group_id query param is required"})
		return
	}
	userID := gc.resolveUserID(c, "")
	if err := gc.service.DeleteFileGroup(c.Request.Context(), userID, groupID); err != nil {
		respondResourceError(c, "failed to delete file group", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ==================== 用户 ID 解析 ====================

func (gc *GraphController) RenameFile(c *gin.Context) {
	var req struct {
		FileID  string `json:"file_id" binding:"required"`
		NewName string `json:"new_name" binding:"required"`
		UserID  string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, req.UserID)
	if err := gc.service.RenameFile(c.Request.Context(), userID, req.FileID, req.NewName); err != nil {
		respondResourceError(c, "failed to rename file", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "renamed"})
}

func (gc *GraphController) RenameFileGroup(c *gin.Context) {
	var req struct {
		GroupID string `json:"group_id" binding:"required"`
		NewName string `json:"new_name" binding:"required"`
		UserID  string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, req.UserID)
	if err := gc.service.RenameFileGroup(c.Request.Context(), userID, req.GroupID, req.NewName); err != nil {
		respondResourceError(c, "failed to rename file group", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "renamed"})
}

func (gc *GraphController) AddFileToGroup(c *gin.Context) {
	var req struct {
		FileID  string `json:"file_id" binding:"required"`
		GroupID string `json:"group_id" binding:"required"`
		UserID  string `json:"user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, req.UserID)
	if err := gc.service.AddFileToGroup(c.Request.Context(), userID, req.FileID, req.GroupID); err != nil {
		respondResourceError(c, "failed to add file to group", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "added"})
}

func (gc *GraphController) TogglePinFile(c *gin.Context) {
	fileID := strings.TrimSpace(c.Query("file_id"))
	if fileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_id query param is required"})
		return
	}
	userID := gc.resolveUserID(c, "")
	if err := gc.service.TogglePinFile(c.Request.Context(), userID, fileID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to toggle pin", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (gc *GraphController) TogglePinFileGroup(c *gin.Context) {
	groupID := strings.TrimSpace(c.Query("group_id"))
	if groupID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group_id query param is required"})
		return
	}
	userID := gc.resolveUserID(c, "")
	if err := gc.service.TogglePinFileGroup(c.Request.Context(), userID, groupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to toggle pin", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ==================== 对话管理 ====================

func (gc *GraphController) GetConversation(c *gin.Context) {
	fileID := strings.TrimSpace(c.Query("file_id"))
	fileGroupID := strings.TrimSpace(c.Query("file_group_id"))
	convID := strings.TrimSpace(c.Query("conversation_id"))

	userID := gc.resolveUserID(c, "")

	if convID != "" {
		conv, err := gc.service.GetConversation(c.Request.Context(), userID, convID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get conversation", "detail": err.Error()})
			return
		}
		c.JSON(http.StatusOK, conv)
		return
	}

	conv, err := gc.service.GetOrCreateConversation(c.Request.Context(), userID, fileID, fileGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get or create conversation", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, conv)
}

func (gc *GraphController) SaveMessage(c *gin.Context) {
	var req model.SaveMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, "")
	if err := gc.service.SaveMessage(c.Request.Context(), req, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save message", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "saved"})
}

func (gc *GraphController) DeleteConversation(c *gin.Context) {
	convID := strings.TrimSpace(c.Query("conversation_id"))
	if convID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation_id query param is required"})
		return
	}
	userID := gc.resolveUserID(c, "")
	if err := gc.service.DeleteConversation(c.Request.Context(), userID, convID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete conversation", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// respondResourceError 把仓储层「资源不存在」的错误映射为 404，
// 其余错误仍按 500 返回。
//
// 背景：删除/重命名/更新接口原先无论什么错误都回 500。
// 而「找不到该文件/文件组」是客户端的请求问题（多半是前端持有已失效的 id），
// 按 500 返回会把它伪装成服务端故障，前端也无法据此提示「该文件已不存在」。
func respondResourceError(c *gin.Context, publicMsg string, err error) {
	if isNotFoundErr(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": publicMsg, "detail": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": publicMsg, "detail": err.Error()})
}

// isNotFoundErr 判断仓储层返回的是否为「资源不存在」。
// 仓储层统一用 fmt.Errorf("%s not found: %s", kind, id) 构造这类错误。
func isNotFoundErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

func (gc *GraphController) resolveUserID(c *gin.Context, bodyUserID string) string {
	if value := strings.TrimSpace(bodyUserID); value != "" {
		return value
	}
	if value := strings.TrimSpace(c.GetHeader("X-User-ID")); value != "" {
		return value
	}
	if value := strings.TrimSpace(c.Query("user_id")); value != "" {
		return value
	}
	return gc.defaultUserID
}

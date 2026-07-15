package controller

import (
	"net/http"
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

func (gc *GraphController) CreateFile(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		FileGroupID string `json:"file_group_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, "")
	fileID, err := gc.service.CreateFile(c.Request.Context(), userID, req.Name, req.FileGroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create file", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"file_id": fileID, "name": req.Name})
}

func (gc *GraphController) CreateFileGroup(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}
	userID := gc.resolveUserID(c, "")
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete file", "detail": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete file group", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ==================== 用户 ID 解析 ====================

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

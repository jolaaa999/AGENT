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
	return &GraphController{
		service:       graphService,
		defaultUserID: defaultUserID,
	}
}

func (gc *GraphController) UploadNote(c *gin.Context) {
	var req model.UploadNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "detail": err.Error()})
		return
	}

	userID := gc.resolveUserID(c, req.UserID)
	result, err := gc.service.UploadNote(c.Request.Context(), req, userID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to upload note", "detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (gc *GraphController) GetGraphAll(c *gin.Context) {
	userID := gc.resolveUserID(c, "")
	graph, err := gc.service.GetGraphAll(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch graph", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, graph)
}

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

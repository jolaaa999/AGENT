package router

import (
	"net/http"
	"time"

	"backend-go/internal/config"
	"backend-go/internal/controller"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func New(cfg config.Config, graphController *controller.GraphController) *gin.Engine {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-User-ID"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "backend-go",
			"status":  "ok",
			"port":    cfg.Port,
		})
	})

	// 图谱核心
	r.POST("/upload-note", graphController.UploadNote)
	r.POST("/upload-note-langchain", graphController.UploadNoteLangChain)
	r.GET("/graph/all", graphController.GetGraphAll)
	r.GET("/graph/path", graphController.GetGraphPath)
	r.GET("/graph/neighbors", graphController.GetNodeNeighbors)
	r.POST("/graph/explain", graphController.ExplainConcept)

	// AI 对话
	r.POST("/graph/chat", graphController.ChatWithContext)
	r.POST("/graph/learning-path", graphController.LearningPath)

	// 文件管理
	r.GET("/files", graphController.ListUserFiles)
	r.POST("/files/create", graphController.CreateFile)
	r.POST("/files/group/create", graphController.CreateFileGroup)
	r.DELETE("/files/delete", graphController.DeleteFile)
	r.DELETE("/files/group/delete", graphController.DeleteFileGroup)

	return r
}

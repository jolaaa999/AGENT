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

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "backend-go",
			"status":  "ok",
			"port":    cfg.Port,
		})
	})

	r.POST("/upload-note", graphController.UploadNote)
	r.GET("/graph/all", graphController.GetGraphAll)
	r.GET("/graph/path", graphController.GetGraphPath)
	r.POST("/graph/explain", graphController.ExplainConcept)

	return r
}

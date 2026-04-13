package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type AppConfig struct {
	Port      string
	Neo4jURI  string
	Neo4jUser string
	Neo4jPass string
}

func loadConfig() AppConfig {
	return AppConfig{
		Port:      getEnv("PORT", "8080"),
		Neo4jURI:  getEnv("NEO4J_URI", "neo4j://localhost:7687"),
		Neo4jUser: getEnv("NEO4J_USERNAME", "neo4j"),
		Neo4jPass: getEnv("NEO4J_PASSWORD", "password"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	cfg := loadConfig()

	driver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		log.Fatalf("failed to create neo4j driver: %v", err)
	}
	defer func() {
		_ = driver.Close(context.Background())
	}()

	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "backend-go-gateway",
			"status":  "ok",
		})
	})

	router.GET("/health/neo4j", func(c *gin.Context) {
		err = driver.VerifyConnectivity(context.Background())
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"service": "neo4j",
				"status":  "down",
				"error":   err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"service": "neo4j",
			"status":  "ok",
		})
	})

	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("failed to start backend-go gateway: %v", err)
	}
}

package main

import (
	"context"
	"log"

	"backend-go/internal/config"
	"backend-go/internal/controller"
	"backend-go/internal/repository"
	"backend-go/internal/router"
	"backend-go/internal/service"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func main() {
	cfg := config.Load()

	driver, err := neo4j.NewDriverWithContext(
		cfg.Neo4jURI,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPassword, ""),
	)
	if err != nil {
		log.Fatalf("failed to create neo4j driver: %v", err)
	}
	defer func() {
		_ = driver.Close(context.Background())
	}()

	if err := driver.VerifyConnectivity(context.Background()); err != nil {
		log.Fatalf("failed to verify neo4j connectivity: %v", err)
	}

	graphRepo := repository.NewGraphRepository(driver)
	graphService := service.NewGraphService(cfg, graphRepo)
	graphController := controller.NewGraphController(graphService, cfg.DefaultUserID)

	ginRouter := router.New(cfg, graphController)
	if err := ginRouter.Run(":" + cfg.Port); err != nil {
		log.Fatalf("failed to start backend-go server: %v", err)
	}
}

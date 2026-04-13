package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port             string
	Neo4jURI         string
	Neo4jUser        string
	Neo4jPassword    string
	PythonServiceURL string
	PythonTimeoutSec int
	DefaultUserID    string
}

func Load() Config {
	return Config{
		Port:             getEnv("PORT", "8080"),
		Neo4jURI:         getEnv("NEO4J_URI", "neo4j://localhost:7687"),
		Neo4jUser:        getEnv("NEO4J_USERNAME", "neo4j"),
		Neo4jPassword:    getEnv("NEO4J_PASSWORD", "password"),
		PythonServiceURL: getEnv("PYTHON_SERVICE_URL", "http://localhost:8000"),
		PythonTimeoutSec: getEnvAsInt("PYTHON_PARSE_TIMEOUT_SECONDS", 120),
		DefaultUserID:    getEnv("DEFAULT_USER_ID", "default_user"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"backend-go/internal/config"
	"backend-go/internal/model"
	"backend-go/internal/repository"
)

type GraphService interface {
	UploadNote(ctx context.Context, req model.UploadNoteRequest, userID string) (map[string]interface{}, error)
	GetGraphAll(ctx context.Context, userID string) (model.G6GraphResponse, error)
	GetGraphPath(ctx context.Context, userID, concept string, maxDepth int) (model.PathResponse, error)
}

type graphService struct {
	cfg        config.Config
	repository repository.GraphRepository
	client     *http.Client
}

func NewGraphService(cfg config.Config, repo repository.GraphRepository) GraphService {
	return &graphService{
		cfg:        cfg,
		repository: repo,
		client: &http.Client{
			Timeout: time.Duration(cfg.PythonTimeoutSec) * time.Second,
		},
	}
}

func (s *graphService) UploadNote(ctx context.Context, req model.UploadNoteRequest, userID string) (map[string]interface{}, error) {
	parseReq := model.ParseRequest{
		Markdown: req.Markdown,
		UserID:   userID,
	}
	payload, err := json.Marshal(parseReq)
	if err != nil {
		return nil, fmt.Errorf("marshal parse request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(s.cfg.PythonServiceURL, "/")+"/api/parse",
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("create python parse request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request python parse service: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read parse response body: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("python parse service error (%d): %s", resp.StatusCode, string(body))
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal parse response: %w", err)
	}

	graphData := normalizeGraphData(raw)
	if err := s.repository.UpsertGraph(ctx, userID, graphData); err != nil {
		return nil, fmt.Errorf("persist graph data into neo4j: %w", err)
	}

	return map[string]interface{}{
		"user_id":         userID,
		"entities_count":  len(graphData.Entities),
		"relations_count": len(graphData.Relations),
		"parser_result":   raw,
	}, nil
}

func (s *graphService) GetGraphAll(ctx context.Context, userID string) (model.G6GraphResponse, error) {
	return s.repository.GetGraphAll(ctx, userID)
}

func (s *graphService) GetGraphPath(ctx context.Context, userID, concept string, maxDepth int) (model.PathResponse, error) {
	return s.repository.GetPathsToConcept(ctx, userID, concept, maxDepth)
}

func normalizeGraphData(raw map[string]interface{}) model.GraphData {
	entitiesRaw := firstArray(raw, "entities", "nodes", "concepts")
	relationsRaw := firstArray(raw, "relations", "edges", "links")

	entities := make([]model.Entity, 0, len(entitiesRaw))
	for _, item := range entitiesRaw {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		entities = append(entities, model.Entity{
			Name:       firstString(obj, "name", "id", "label"),
			Type:       firstString(obj, "type", "category"),
			Status:     firstString(obj, "status"),
			Reason:     firstString(obj, "reason"),
			Properties: obj,
		})
	}

	relations := make([]model.Relation, 0, len(relationsRaw))
	for _, item := range relationsRaw {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		relations = append(relations, model.Relation{
			Source:     firstString(obj, "source", "from"),
			Target:     firstString(obj, "target", "to"),
			Type:       firstString(obj, "type", "relation"),
			Status:     firstString(obj, "status"),
			Reason:     firstString(obj, "reason"),
			Properties: obj,
		})
	}

	return model.GraphData{
		Entities:  entities,
		Relations: relations,
	}
}

func firstArray(obj map[string]interface{}, keys ...string) []interface{} {
	for _, key := range keys {
		value, exists := obj[key]
		if !exists {
			continue
		}
		if arr, ok := value.([]interface{}); ok {
			return arr
		}
	}
	return []interface{}{}
}

func firstString(obj map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, exists := obj[key]
		if !exists || value == nil {
			continue
		}
		if s, ok := value.(string); ok {
			return s
		}
		return fmt.Sprintf("%v", value)
	}
	return ""
}

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
	ExplainConcept(ctx context.Context, req model.ExplainRequest, userID string) (model.ExplainResponse, error)
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

	var parseResp model.ParseResponse
	if err := json.Unmarshal(body, &parseResp); err != nil {
		return nil, fmt.Errorf("unmarshal parse response: %w", err)
	}

	graphData := normalizeGraphData(parseResp)
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

func (s *graphService) GetGraphAll(ctx context.Context, userID string) (model.G6GraphResponse, error) {
	return s.repository.GetGraphAll(ctx, userID)
}

func (s *graphService) GetGraphPath(ctx context.Context, userID, concept string, maxDepth int) (model.PathResponse, error) {
	return s.repository.GetPathsToConcept(ctx, userID, concept, maxDepth)
}

func (s *graphService) ExplainConcept(ctx context.Context, req model.ExplainRequest, userID string) (model.ExplainResponse, error) {
	payload, err := json.Marshal(map[string]string{
		"concept":  strings.TrimSpace(req.Concept),
		"markdown": req.Markdown,
	})
	if err != nil {
		return model.ExplainResponse{}, fmt.Errorf("marshal explain request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(s.cfg.PythonServiceURL, "/")+"/api/explain",
		bytes.NewReader(payload),
	)
	if err != nil {
		return model.ExplainResponse{}, fmt.Errorf("create python explain request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return model.ExplainResponse{}, fmt.Errorf("request python explain service: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return model.ExplainResponse{}, fmt.Errorf("read explain response body: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return model.ExplainResponse{}, fmt.Errorf("python explain service error (%d): %s", resp.StatusCode, string(body))
	}

	var explainResp model.ExplainResponse
	if err := json.Unmarshal(body, &explainResp); err != nil {
		return model.ExplainResponse{}, fmt.Errorf("unmarshal explain response: %w", err)
	}
	return explainResp, nil
}

func normalizeGraphData(parseResp model.ParseResponse) model.GraphData {
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
				Name:       source,
				Type:       "Concept",
				Properties: map[string]interface{}{},
			}
		}
		if _, exists := entityMap[target]; !exists {
			entityMap[target] = model.Entity{
				Name:       target,
				Type:       "Concept",
				Status:     rel.Status,
				Reason:     rel.Reason,
				Properties: map[string]interface{}{},
			}
		}

		relations = append(relations, model.Relation{
			Source:      source,
			Target:      target,
			Type:        rel.Relation,
			Description: rel.Relation,
			Status:      rel.Status,
			Reason:      rel.Reason,
			Properties: map[string]interface{}{
				"relation": rel.Relation,
			},
		})
	}

	entities := make([]model.Entity, 0, len(entityMap))
	for _, entity := range entityMap {
		entities = append(entities, entity)
	}

	return model.GraphData{Entities: entities, Relations: relations}
}

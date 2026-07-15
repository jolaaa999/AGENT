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
	// 图谱核心
	UploadNote(ctx context.Context, req model.UploadNoteRequest, userID string) (map[string]interface{}, error)
	UploadNoteLangChain(ctx context.Context, req model.UploadNoteRequest, userID string) (map[string]interface{}, error)
	GetGraphAll(ctx context.Context, userID string) (model.G6GraphResponse, error)
	GetGraphByFile(ctx context.Context, userID, fileID string) (model.G6GraphResponse, error)
	GetGraphByFileGroup(ctx context.Context, userID, fileGroupID string) (model.G6GraphResponse, error)
	GetGraphPath(ctx context.Context, userID, concept string, maxDepth int) (model.PathResponse, error)
	ExplainConcept(ctx context.Context, req model.ExplainRequest, userID string) (model.ExplainResponse, error)
	GetNodeNeighbors(ctx context.Context, userID string, nodeID string, depth int) (model.G6GraphResponse, error)
	// AI 对话
	ChatWithContext(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error)
	LearningPath(ctx context.Context, req model.LearningPathRequest) (model.LearningPathResponse, error)
	// 文件管理
	CreateFile(ctx context.Context, userID, name, fileGroupID string) (string, error)
	CreateFileGroup(ctx context.Context, userID, name string) (string, error)
	ListUserFiles(ctx context.Context, userID string) ([]model.UserFile, []model.FileGroup, error)
	DeleteFile(ctx context.Context, userID, fileID string) error
	DeleteFileGroup(ctx context.Context, userID, groupID string) error
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

// ==================== 图谱上传（原始 DeepSeek SDK） ====================

func (s *graphService) UploadNote(ctx context.Context, req model.UploadNoteRequest, userID string) (map[string]interface{}, error) {
	parseReq := model.ParseRequest{Markdown: req.Markdown}
	payload, err := json.Marshal(parseReq)
	if err != nil {
		return nil, fmt.Errorf("marshal parse request: %w", err)
	}

	body, err := s.callPython(ctx, "POST", "/api/parse", payload)
	if err != nil {
		return nil, err
	}

	var parseResp model.ParseResponse
	if err := json.Unmarshal(body, &parseResp); err != nil {
		return nil, fmt.Errorf("unmarshal parse response: %w", err)
	}

	graphData := normalizeGraphData(parseResp, req.FileID, req.FileGroupID)
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

// ==================== 图谱上传（LangChain 诊断流水线） ====================

func (s *graphService) UploadNoteLangChain(ctx context.Context, req model.UploadNoteRequest, userID string) (map[string]interface{}, error) {
	diagReq := model.LangChainDiagnoseRequest{Markdown: req.Markdown}
	payload, err := json.Marshal(diagReq)
	if err != nil {
		return nil, fmt.Errorf("marshal langchain diagnose request: %w", err)
	}

	body, err := s.callPython(ctx, "POST", "/api/langchain/diagnose", payload)
	if err != nil {
		return nil, err
	}

	var diagResp model.LangChainDiagnoseResponse
	if err := json.Unmarshal(body, &diagResp); err != nil {
		return nil, fmt.Errorf("unmarshal langchain diagnose response: %w", err)
	}
	if !diagResp.Success {
		return nil, fmt.Errorf("langchain diagnose failed: %s", diagResp.ErrorMessage)
	}

	// 将 LangChain 响应转换为 GraphData
	graphData := normalizeLangChainData(diagResp, req.FileID, req.FileGroupID)
	if err := s.repository.UpsertGraph(ctx, userID, graphData); err != nil {
		return nil, fmt.Errorf("persist langchain graph data into neo4j: %w", err)
	}

	// 统计各状态数量
	errorCount := 0
	supplementCount := 0
	for _, entity := range graphData.Entities {
		switch entity.Status {
		case "error":
			errorCount++
		case "supplement":
			supplementCount++
		}
	}

	return map[string]interface{}{
		"user_id":          userID,
		"entities_count":   len(graphData.Entities),
		"relations_count":  len(graphData.Relations),
		"error_count":      errorCount,
		"supplement_count": supplementCount,
		"summary":          diagResp.Summary,
		"retries_used":     diagResp.RetriesUsed,
		"diagnose_result":  diagResp,
	}, nil
}

// ==================== 图谱查询 ====================

func (s *graphService) GetGraphAll(ctx context.Context, userID string) (model.G6GraphResponse, error) {
	return s.repository.GetGraphAll(ctx, userID)
}

func (s *graphService) GetGraphByFile(ctx context.Context, userID, fileID string) (model.G6GraphResponse, error) {
	return s.repository.GetGraphByFile(ctx, userID, fileID)
}

func (s *graphService) GetGraphByFileGroup(ctx context.Context, userID, fileGroupID string) (model.G6GraphResponse, error) {
	return s.repository.GetGraphByFileGroup(ctx, userID, fileGroupID)
}

func (s *graphService) GetGraphPath(ctx context.Context, userID, concept string, maxDepth int) (model.PathResponse, error) {
	return s.repository.GetPathsToConcept(ctx, userID, concept, maxDepth)
}

func (s *graphService) GetNodeNeighbors(ctx context.Context, userID string, nodeID string, depth int) (model.G6GraphResponse, error) {
	return s.repository.GetNodeNeighbors(ctx, userID, nodeID, depth)
}

// ==================== 概念讲解 ====================

func (s *graphService) ExplainConcept(ctx context.Context, req model.ExplainRequest, userID string) (model.ExplainResponse, error) {
	payload, err := json.Marshal(map[string]string{
		"concept":  strings.TrimSpace(req.Concept),
		"markdown": req.Markdown,
	})
	if err != nil {
		return model.ExplainResponse{}, fmt.Errorf("marshal explain request: %w", err)
	}

	body, err := s.callPython(ctx, "POST", "/api/explain", payload)
	if err != nil {
		return model.ExplainResponse{}, err
	}

	var explainResp model.ExplainResponse
	if err := json.Unmarshal(body, &explainResp); err != nil {
		return model.ExplainResponse{}, fmt.Errorf("unmarshal explain response: %w", err)
	}
	return explainResp, nil
}

// ==================== AI 对话 ====================

func (s *graphService) ChatWithContext(ctx context.Context, req model.ChatRequest) (model.ChatResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return model.ChatResponse{}, fmt.Errorf("marshal chat request: %w", err)
	}

	body, err := s.callPython(ctx, "POST", "/api/langchain/chat", payload)
	if err != nil {
		return model.ChatResponse{}, err
	}

	var chatResp model.ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return model.ChatResponse{}, fmt.Errorf("unmarshal chat response: %w", err)
	}
	return chatResp, nil
}

func (s *graphService) LearningPath(ctx context.Context, req model.LearningPathRequest) (model.LearningPathResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return model.LearningPathResponse{}, fmt.Errorf("marshal learning path request: %w", err)
	}

	body, err := s.callPython(ctx, "POST", "/api/langchain/learning-path", payload)
	if err != nil {
		return model.LearningPathResponse{}, err
	}

	var pathResp model.LearningPathResponse
	if err := json.Unmarshal(body, &pathResp); err != nil {
		return model.LearningPathResponse{}, fmt.Errorf("unmarshal learning path response: %w", err)
	}
	return pathResp, nil
}

// ==================== 文件管理 ====================

func (s *graphService) CreateFile(ctx context.Context, userID, name, fileGroupID string) (string, error) {
	return s.repository.CreateFile(ctx, userID, name, fileGroupID)
}

func (s *graphService) CreateFileGroup(ctx context.Context, userID, name string) (string, error) {
	return s.repository.CreateFileGroup(ctx, userID, name)
}

func (s *graphService) ListUserFiles(ctx context.Context, userID string) ([]model.UserFile, []model.FileGroup, error) {
	return s.repository.ListUserFiles(ctx, userID)
}

func (s *graphService) DeleteFile(ctx context.Context, userID, fileID string) error {
	return s.repository.DeleteFile(ctx, userID, fileID)
}

func (s *graphService) DeleteFileGroup(ctx context.Context, userID, groupID string) error {
	return s.repository.DeleteFileGroup(ctx, userID, groupID)
}

// ==================== 内部辅助 ====================

func (s *graphService) callPython(ctx context.Context, method, path string, payload []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(
		ctx, method,
		strings.TrimRight(s.cfg.PythonServiceURL, "/")+path,
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("create python request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request python service %s: %w", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("python service error (%d) at %s: %s", resp.StatusCode, path, string(body))
	}
	return body, nil
}

// normalizeGraphData 将旧版 ParseResponse 转换为 GraphData
func normalizeGraphData(parseResp model.ParseResponse, fileID, fileGroupID string) model.GraphData {
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
				Name: source, Type: "Concept",
				FileID: fileID, FileGroupID: fileGroupID,
				Properties: map[string]interface{}{},
			}
		}
		if _, exists := entityMap[target]; !exists {
			entityMap[target] = model.Entity{
				Name: target, Type: "Concept",
				Status: rel.Status, Reason: rel.Reason,
				FileID: fileID, FileGroupID: fileGroupID,
				Properties: map[string]interface{}{},
			}
		}

		relations = append(relations, model.Relation{
			Source: source, Target: target,
			Type: rel.Relation, Description: rel.Relation,
			Status: rel.Status, Reason: rel.Reason,
			Properties: map[string]interface{}{"relation": rel.Relation},
		})
	}

	entities := make([]model.Entity, 0, len(entityMap))
	for _, entity := range entityMap {
		entities = append(entities, entity)
	}
	return model.GraphData{Entities: entities, Relations: relations}
}

// normalizeLangChainData 将 LangChain 诊断结果转换为 GraphData
func normalizeLangChainData(diag model.LangChainDiagnoseResponse, fileID, fileGroupID string) model.GraphData {
	entityMap := map[string]model.Entity{}
	relations := make([]model.Relation, 0, len(diag.Edges))

	for _, node := range diag.Nodes {
		name := strings.TrimSpace(node.Name)
		if name == "" {
			continue
		}
		entityMap[name] = model.Entity{
			Name: name, Type: fallbackS(node.EntityType, "Concept"),
			Status: node.Status, Reason: node.Reason,
			FileID: fileID, FileGroupID: fileGroupID,
			Properties: map[string]interface{}{
				"definition": node.Definition,
				"source":     node.Source,
			},
		}
	}

	for _, edge := range diag.Edges {
		source := strings.TrimSpace(edge.Source)
		target := strings.TrimSpace(edge.Target)
		if source == "" || target == "" {
			continue
		}
		// 确保源和目标实体存在
		if _, exists := entityMap[source]; !exists {
			entityMap[source] = model.Entity{
				Name: source, Type: "Concept",
				FileID: fileID, FileGroupID: fileGroupID,
				Properties: map[string]interface{}{},
			}
		}
		if _, exists := entityMap[target]; !exists {
			entityMap[target] = model.Entity{
				Name: target, Type: "Concept",
				FileID: fileID, FileGroupID: fileGroupID,
				Properties: map[string]interface{}{},
			}
		}

		relations = append(relations, model.Relation{
			Source: source, Target: target,
			Type: fallbackS(edge.Relation, "RELATED_TO"),
			Description: edge.Relation,
			Status: edge.Status, Reason: edge.Reason,
			Properties: map[string]interface{}{
				"relation":     edge.Relation,
				"source_agent": edge.SourceAgent,
			},
		})
	}

	entities := make([]model.Entity, 0, len(entityMap))
	for _, entity := range entityMap {
		entities = append(entities, entity)
	}
	return model.GraphData{Entities: entities, Relations: relations}
}

func fallbackS(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

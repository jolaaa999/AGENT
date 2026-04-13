package repository

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"backend-go/internal/model"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type GraphRepository interface {
	UpsertGraph(ctx context.Context, userID string, data model.GraphData) error
	GetGraphAll(ctx context.Context, userID string) (model.G6GraphResponse, error)
	GetPathsToConcept(ctx context.Context, userID, concept string, maxDepth int) (model.PathResponse, error)
}

type graphRepository struct {
	driver neo4j.DriverWithContext
}

var relTypeSanitizer = regexp.MustCompile(`[^A-Z0-9_]`)

func NewGraphRepository(driver neo4j.DriverWithContext) GraphRepository {
	return &graphRepository{driver: driver}
}

func (r *graphRepository) UpsertGraph(ctx context.Context, userID string, data model.GraphData) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		for _, entity := range data.Entities {
			if strings.TrimSpace(entity.Name) == "" {
				continue
			}
			params := map[string]interface{}{
				"user_id":    userID,
				"name":       entity.Name,
				"type":       fallback(entity.Type, "Concept"),
				"status":     entity.Status,
				"reason":     entity.Reason,
				"extraProps": sanitizeProps(entity.Properties),
				"updated_at": time.Now().UTC().Format(time.RFC3339),
			}
			if _, err := tx.Run(ctx, `
				MERGE (n:Concept {user_id: $user_id, name: $name})
				SET n.type = $type,
				    n.status = $status,
				    n.reason = $reason,
				    n.updated_at = $updated_at
				SET n += $extraProps
			`, params); err != nil {
				return nil, err
			}
		}

		for _, rel := range data.Relations {
			if strings.TrimSpace(rel.Source) == "" || strings.TrimSpace(rel.Target) == "" {
				continue
			}
			relType := sanitizeRelType(rel.Type)
			query := fmt.Sprintf(`
				MERGE (s:Concept {user_id: $user_id, name: $source})
				ON CREATE SET s.type = "Concept"
				MERGE (t:Concept {user_id: $user_id, name: $target})
				ON CREATE SET t.type = "Concept"
				SET t.status = $status,
				    t.reason = $reason,
				    t.last_relation = $relation_desc,
				    t.updated_at = $updated_at
				MERGE (s)-[r:%s {user_id: $user_id}]->(t)
				SET r.status = $status,
				    r.reason = $reason,
				    r.description = $relation_desc,
				    r.updated_at = $updated_at
				SET r += $extraProps
			`, relType)

			params := map[string]interface{}{
				"user_id":    userID,
				"source":     rel.Source,
				"target":     rel.Target,
				"status":     rel.Status,
				"reason":     rel.Reason,
				"relation_desc": fallback(rel.Description, rel.Type),
				"extraProps": sanitizeProps(rel.Properties),
				"updated_at": time.Now().UTC().Format(time.RFC3339),
			}
			if _, err := tx.Run(ctx, query, params); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})

	return err
}

func (r *graphRepository) GetGraphAll(ctx context.Context, userID string) (model.G6GraphResponse, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	nodes := make([]model.G6Node, 0)
	edges := make([]model.G6Edge, 0)

	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		nodeResult, err := tx.Run(ctx, `
			MATCH (n:Concept {user_id: $user_id})
			RETURN n
			ORDER BY n.name
		`, map[string]interface{}{"user_id": userID})
		if err != nil {
			return nil, err
		}
		for nodeResult.Next(ctx) {
			record := nodeResult.Record()
			nodeValue, _ := record.Get("n")
			node, ok := nodeValue.(neo4j.Node)
			if !ok {
				continue
			}
			props := node.Props
			name := asString(props["name"])
			nodes = append(nodes, model.G6Node{
				ID:     name,
				Label:  name,
				Type:   asString(props["type"]),
				Status: asString(props["status"]),
				Reason: asString(props["reason"]),
				Data:   props,
			})
		}
		if err := nodeResult.Err(); err != nil {
			return nil, err
		}

		edgeResult, err := tx.Run(ctx, `
			MATCH (s:Concept {user_id: $user_id})-[r]->(t:Concept {user_id: $user_id})
			RETURN s.name AS source, t.name AS target, type(r) AS rel_type, properties(r) AS props
			ORDER BY source, target
		`, map[string]interface{}{"user_id": userID})
		if err != nil {
			return nil, err
		}
		for edgeResult.Next(ctx) {
			record := edgeResult.Record()
			source := asString(record.Values[0])
			target := asString(record.Values[1])
			relType := asString(record.Values[2])
			props, _ := record.Values[3].(map[string]interface{})
			edges = append(edges, model.G6Edge{
				ID:     fmt.Sprintf("%s-%s-%s", source, relType, target),
				Source: source,
				Target: target,
				Label:  relType,
				Status: asString(props["status"]),
				Reason: asString(props["reason"]),
				Data:   props,
			})
		}
		return nil, edgeResult.Err()
	})
	if err != nil {
		return model.G6GraphResponse{}, err
	}

	return model.G6GraphResponse{Nodes: nodes, Edges: edges}, nil
}

func (r *graphRepository) GetPathsToConcept(ctx context.Context, userID, concept string, maxDepth int) (model.PathResponse, error) {
	if maxDepth < 1 {
		maxDepth = 3
	}
	if maxDepth > 6 {
		maxDepth = 6
	}

	query := fmt.Sprintf(`
		MATCH path=(n:Concept {user_id: $user_id})-[*1..%d]->(m:Concept {user_id: $user_id, name: $concept})
		RETURN path
	`, maxDepth)

	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	pathGraphs := make([]model.G6GraphResponse, 0)
	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, map[string]interface{}{
			"user_id": userID,
			"concept": concept,
		})
		if err != nil {
			return nil, err
		}

		for result.Next(ctx) {
			record := result.Record()
			pathVal, _ := record.Get("path")
			path, ok := pathVal.(neo4j.Path)
			if !ok {
				continue
			}
			pathGraphs = append(pathGraphs, convertPathToG6(path))
		}
		return nil, result.Err()
	})
	if err != nil {
		return model.PathResponse{}, err
	}

	return model.PathResponse{
		Concept: concept,
		Paths:   pathGraphs,
	}, nil
}

func convertPathToG6(path neo4j.Path) model.G6GraphResponse {
	nodes := make([]model.G6Node, 0, len(path.Nodes))
	edges := make([]model.G6Edge, 0, len(path.Relationships))

	nodeSeen := make(map[string]struct{})
	for _, n := range path.Nodes {
		name := asString(n.Props["name"])
		if _, exists := nodeSeen[name]; exists {
			continue
		}
		nodeSeen[name] = struct{}{}
		nodes = append(nodes, model.G6Node{
			ID:     name,
			Label:  name,
			Type:   asString(n.Props["type"]),
			Status: asString(n.Props["status"]),
			Reason: asString(n.Props["reason"]),
			Data:   n.Props,
		})
	}

	for _, rel := range path.Relationships {
		source := findNodeNameByID(path.Nodes, rel.StartId)
		target := findNodeNameByID(path.Nodes, rel.EndId)
		edges = append(edges, model.G6Edge{
			ID:     fmt.Sprintf("%s-%s-%s", source, rel.Type, target),
			Source: source,
			Target: target,
			Label:  rel.Type,
			Status: asString(rel.Props["status"]),
			Reason: asString(rel.Props["reason"]),
			Data:   rel.Props,
		})
	}

	return model.G6GraphResponse{
		Nodes: nodes,
		Edges: edges,
	}
}

func findNodeNameByID(nodes []neo4j.Node, id int64) string {
	for _, node := range nodes {
		if node.Id == id {
			return asString(node.Props["name"])
		}
	}
	return ""
}

func sanitizeRelType(relType string) string {
	relType = strings.TrimSpace(relType)
	if relType == "" {
		return "RELATED_TO"
	}
	upper := strings.ToUpper(strings.ReplaceAll(relType, " ", "_"))
	upper = relTypeSanitizer.ReplaceAllString(upper, "_")
	if upper == "" {
		return "RELATED_TO"
	}
	return upper
}

func sanitizeProps(props map[string]interface{}) map[string]interface{} {
	if props == nil {
		return map[string]interface{}{}
	}
	safe := make(map[string]interface{}, len(props))
	for k, v := range props {
		if strings.EqualFold(k, "user_id") || strings.EqualFold(k, "name") {
			continue
		}
		safe[k] = v
	}
	return safe
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}

func asString(value interface{}) string {
	if value == nil {
		return ""
	}
	s, ok := value.(string)
	if ok {
		return s
	}
	return fmt.Sprintf("%v", value)
}

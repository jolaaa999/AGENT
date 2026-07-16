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
	GetGraphByFile(ctx context.Context, userID, fileID string) (model.G6GraphResponse, error)
	GetGraphByFileGroup(ctx context.Context, userID, fileGroupID string) (model.G6GraphResponse, error)
	GetPathsToConcept(ctx context.Context, userID, concept string, maxDepth int) (model.PathResponse, error)
	GetNodeNeighbors(ctx context.Context, userID string, nodeID string, depth int) (model.G6GraphResponse, error)
	// 文件管理
	CreateFile(ctx context.Context, userID, name, fileGroupID string) (string, error)
	CreateFileGroup(ctx context.Context, userID, name string) (string, error)
	ListUserFiles(ctx context.Context, userID string) ([]model.UserFile, []model.FileGroup, error)
	DeleteFile(ctx context.Context, userID, fileID string) error
	DeleteFileGroup(ctx context.Context, userID, groupID string) error
	RenameFile(ctx context.Context, userID, fileID, newName string) error
	RenameFileGroup(ctx context.Context, userID, groupID, newName string) error
	AddFileToGroup(ctx context.Context, userID, fileID, groupID string) error
	TogglePinFile(ctx context.Context, userID, fileID string) error
	TogglePinFileGroup(ctx context.Context, userID, groupID string) error
	// 对话管理
	GetOrCreateConversation(ctx context.Context, userID, fileID, fileGroupID string) (model.Conversation, error)
	SaveMessage(ctx context.Context, req model.SaveMessageRequest, userID string) error
	GetConversation(ctx context.Context, userID, conversationID string) (model.Conversation, error)
	DeleteConversation(ctx context.Context, userID, conversationID string) error
}

type graphRepository struct {
	driver neo4j.DriverWithContext
}

var relTypeSanitizer = regexp.MustCompile(`[^A-Z0-9_]`)

func NewGraphRepository(driver neo4j.DriverWithContext) GraphRepository {
	return &graphRepository{driver: driver}
}

// ==================== 图谱写入 ====================

func (r *graphRepository) UpsertGraph(ctx context.Context, userID string, data model.GraphData) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		for _, entity := range data.Entities {
			if strings.TrimSpace(entity.Name) == "" {
				continue
			}
			params := map[string]interface{}{
				"user_id":       userID,
				"name":          entity.Name,
				"type":          fallback(entity.Type, "Concept"),
				"status":        entity.Status,
				"reason":        entity.Reason,
				"file_id":       entity.FileID,
				"file_group_id": entity.FileGroupID,
				"extraProps":    sanitizeProps(entity.Properties),
				"updated_at":    time.Now().UTC().Format(time.RFC3339),
			}
			if _, err := tx.Run(ctx, `
				MERGE (n:Concept {user_id: $user_id, name: $name})
				SET n.type = $type,
				    n.status = $status,
				    n.reason = $reason,
				    n.file_id = $file_id,
				    n.file_group_id = $file_group_id,
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
				"user_id":       userID,
				"source":        rel.Source,
				"target":        rel.Target,
				"status":        rel.Status,
				"reason":        rel.Reason,
				"relation_desc": fallback(rel.Description, rel.Type),
				"extraProps":    sanitizeProps(rel.Properties),
				"updated_at":    time.Now().UTC().Format(time.RFC3339),
			}
			if _, err := tx.Run(ctx, query, params); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})

	return err
}

// ==================== 图谱查询 ====================

func (r *graphRepository) GetGraphAll(ctx context.Context, userID string) (model.G6GraphResponse, error) {
	return r.queryGraph(ctx, `
		MATCH (n:Concept {user_id: $user_id})
		RETURN n ORDER BY n.name
	`, `
		MATCH (s:Concept {user_id: $user_id})-[r]->(t:Concept {user_id: $user_id})
		RETURN s.name AS source, t.name AS target, type(r) AS rel_type, properties(r) AS props
		ORDER BY source, target
	`, userID)
}

func (r *graphRepository) GetGraphByFile(ctx context.Context, userID, fileID string) (model.G6GraphResponse, error) {
	return r.queryGraph(ctx, `
		MATCH (n:Concept {user_id: $user_id, file_id: $file_id})
		RETURN n ORDER BY n.name
	`, `
		MATCH (s:Concept {user_id: $user_id, file_id: $file_id})-[r]->(t:Concept {user_id: $user_id, file_id: $file_id})
		RETURN s.name AS source, t.name AS target, type(r) AS rel_type, properties(r) AS props
		ORDER BY source, target
	`, userID, map[string]interface{}{"file_id": fileID})
}

func (r *graphRepository) GetGraphByFileGroup(ctx context.Context, userID, fileGroupID string) (model.G6GraphResponse, error) {
	return r.queryGraph(ctx, `
		MATCH (n:Concept {user_id: $user_id, file_group_id: $file_group_id})
		RETURN n ORDER BY n.name
	`, `
		MATCH (s:Concept {user_id: $user_id, file_group_id: $file_group_id})-[r]->(t:Concept {user_id: $user_id, file_group_id: $file_group_id})
		RETURN s.name AS source, t.name AS target, type(r) AS rel_type, properties(r) AS props
		ORDER BY source, target
	`, userID, map[string]interface{}{"file_group_id": fileGroupID})
}

// queryGraph 通用图谱查询（节点+边）
func (r *graphRepository) queryGraph(
	ctx context.Context,
	nodeCypher, edgeCypher string,
	userID string,
	extraParams ...map[string]interface{},
) (model.G6GraphResponse, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	nodes := make([]model.G6Node, 0)
	edges := make([]model.G6Edge, 0)

	params := map[string]interface{}{"user_id": userID}
	if len(extraParams) > 0 {
		for k, v := range extraParams[0] {
			params[k] = v
		}
	}

	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// 查询节点
		nodeResult, err := tx.Run(ctx, nodeCypher, params)
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
				FileID: asString(props["file_id"]),
				Data:   props,
			})
		}
		if err := nodeResult.Err(); err != nil {
			return nil, err
		}

		// 查询边
		edgeResult, err := tx.Run(ctx, edgeCypher, params)
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

// ==================== DFS 逆向技能树查询 ====================

func (r *graphRepository) GetPathsToConcept(ctx context.Context, userID, concept string, maxDepth int) (model.PathResponse, error) {
	if maxDepth < 1 {
		maxDepth = 3
	}
	if maxDepth > 6 {
		maxDepth = 6
	}

	// 逆向 DFS：从目标概念反向追溯所有 PREREQUISITE_OF 前置依赖
	// 方向：前置节点 -[PREREQUISITE_OF]-> 目标节点
	// 逆向查询：目标概念 <-[:PREREQUISITE_OF*1..N]- 前置依赖
	reverseQuery := fmt.Sprintf(`
		MATCH (m:Concept {user_id: $user_id, name: $concept})
		// 逆向匹配：从目标概念出发，沿入边反向追溯前置依赖
		// 使用 incoming relationships 来实现"逆向"语义
		MATCH path=(prereq:Concept {user_id: $user_id})-[r*1..%d]->(m)
		WHERE all(rel IN r WHERE type(rel) = 'PREREQUISITE_OF')
		RETURN prereq, r, length(path) AS depth
		ORDER BY depth
	`, maxDepth)

	// 同时查询目标概念的所有直接关联节点（用于前端专注模式的高亮）
	// 返回 source/target 使用概念名而非 Neo4j 内部 ID，确保前端能匹配节点
	relatedQuery := `
		MATCH (m:Concept {user_id: $user_id, name: $concept})
		MATCH (related:Concept {user_id: $user_id})-[r]-(m)
		RETURN DISTINCT related, r, startNode(r).name AS source_name, endNode(r).name AS target_name, type(r) AS rel_type
	`

	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	dependencyTree := make([]model.DependencyNode, 0)
	allNodes := make(map[string]model.G6Node)
	allEdges := make(map[string]model.G6Edge)

	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// 1. 逆向依赖树查询
		revResult, err := tx.Run(ctx, reverseQuery, map[string]interface{}{
			"user_id": userID,
			"concept": concept,
		})
		if err != nil {
			return nil, err
		}
		for revResult.Next(ctx) {
			record := revResult.Record()
			prereqVal, _ := record.Get("prereq")
			depthVal, _ := record.Get("depth")

			prereqNode, ok := prereqVal.(neo4j.Node)
			if !ok {
				continue
			}
			name := asString(prereqNode.Props["name"])
			depth := 0
			if d, ok := depthVal.(int64); ok {
				depth = int(d)
			}

			dependencyTree = append(dependencyTree, model.DependencyNode{
				Name:   name,
				Depth:  depth,
				Status: asString(prereqNode.Props["status"]),
				Reason: asString(prereqNode.Props["reason"]),
			})

			// 收集节点
			if _, exists := allNodes[name]; !exists {
				allNodes[name] = model.G6Node{
					ID:     name,
					Label:  name,
					Type:   asString(prereqNode.Props["type"]),
					Status: asString(prereqNode.Props["status"]),
					Reason: asString(prereqNode.Props["reason"]),
					Data:   prereqNode.Props,
				}
			}

			// 收集关系
			relsVal, _ := record.Get("r")
			if rels, ok := relsVal.([]interface{}); ok {
				for _, relVal := range rels {
					if rel, ok := relVal.(neo4j.Relationship); ok {
						edgeKey := fmt.Sprintf("%d", rel.Id)
						if _, exists := allEdges[edgeKey]; !exists {
							allEdges[edgeKey] = model.G6Edge{
								ID:     edgeKey,
								Source: asString(prereqNode.Props["name"]),
								Target: concept,
								Label:  rel.Type,
								Status: asString(rel.Props["status"]),
								Reason: asString(rel.Props["reason"]),
								Data:   rel.Props,
							}
						}
					}
				}
			}
		}

		// 2. 目标概念的直接关联节点
		relResult, err := tx.Run(ctx, relatedQuery, map[string]interface{}{
			"user_id": userID,
			"concept": concept,
		})
		if err != nil {
			return nil, err
		}
		for relResult.Next(ctx) {
			record := relResult.Record()
			relatedVal, _ := record.Get("related")
			relVal, _ := record.Get("r")

			if relatedNode, ok := relatedVal.(neo4j.Node); ok {
				name := asString(relatedNode.Props["name"])
				if _, exists := allNodes[name]; !exists {
					allNodes[name] = model.G6Node{
						ID:     name,
						Label:  name,
						Type:   asString(relatedNode.Props["type"]),
						Status: asString(relatedNode.Props["status"]),
						Reason: asString(relatedNode.Props["reason"]),
						Data:   relatedNode.Props,
					}
				}
			}
			if rel, ok := relVal.(neo4j.Relationship); ok {
					sourceName, _ := record.Get("source_name")
					targetName, _ := record.Get("target_name")
					src := asString(sourceName)
					tgt := asString(targetName)
					if src == "" { src = fmt.Sprintf("%d", rel.StartId) }
					if tgt == "" { tgt = fmt.Sprintf("%d", rel.EndId) }
					edgeKey := fmt.Sprintf("%s-%s-%s", src, rel.Type, tgt)
					if _, exists := allEdges[edgeKey]; !exists {
						allEdges[edgeKey] = model.G6Edge{
							ID:     edgeKey,
							Source: src,
							Target: tgt,
							Label:  rel.Type,
							Status: asString(rel.Props["status"]),
							Reason: asString(rel.Props["reason"]),
							Data:   rel.Props,
						}
					}
				}
		}

		return nil, nil
	})
	if err != nil {
		return model.PathResponse{}, err
	}

	// 组装响应
	nodeList := make([]model.G6Node, 0, len(allNodes))
	for _, n := range allNodes {
		nodeList = append(nodeList, n)
	}
	edgeList := make([]model.G6Edge, 0, len(allEdges))
	for _, e := range allEdges {
		edgeList = append(edgeList, e)
	}

	allRelated := &model.G6GraphResponse{Nodes: nodeList, Edges: edgeList}

	return model.PathResponse{
		Concept:        concept,
		Paths:          []model.G6GraphResponse{*allRelated},
		DependencyTree: dependencyTree,
		AllRelated:     allRelated,
	}, nil
}

// ==================== 邻居查询 ====================

func (r *graphRepository) GetNodeNeighbors(ctx context.Context, userID string, nodeID string, depth int) (model.G6GraphResponse, error) {
	if depth < 1 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	nodes := make([]model.G6Node, 0)
	edges := make([]model.G6Edge, 0)
	nodeSeen := make(map[string]struct{})

	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// 获取中心节点
		centerResult, err := tx.Run(ctx, `
			MATCH (n:Concept {user_id: $user_id, name: $node_id})
			RETURN n
		`, map[string]interface{}{"user_id": userID, "node_id": nodeID})
		if err != nil {
			return nil, err
		}
		if centerResult.Next(ctx) {
			nodeValue, _ := centerResult.Record().Get("n")
			if node, ok := nodeValue.(neo4j.Node); ok {
				props := node.Props
				name := asString(props["name"])
				nodes = append(nodes, model.G6Node{
					ID: name, Label: name,
					Type: asString(props["type"]), Status: asString(props["status"]),
					Reason: asString(props["reason"]), Data: props,
				})
				nodeSeen[name] = struct{}{}
			}
		}

		// 双向邻居查询
		query := fmt.Sprintf(`
			MATCH (center:Concept {user_id: $user_id, name: $node_id})-[r*1..%d]-(neighbor:Concept {user_id: $user_id})
			RETURN neighbor, r as relations
		`, depth)

		neighborResult, err := tx.Run(ctx, query, map[string]interface{}{
			"user_id": userID, "node_id": nodeID,
		})
		if err != nil {
			return nil, err
		}

		for neighborResult.Next(ctx) {
			record := neighborResult.Record()
			if neighborNode, ok := record.Values[0].(neo4j.Node); ok {
				props := neighborNode.Props
				name := asString(props["name"])
				if _, exists := nodeSeen[name]; !exists {
					nodes = append(nodes, model.G6Node{
						ID: name, Label: name,
						Type: asString(props["type"]), Status: asString(props["status"]),
						Reason: asString(props["reason"]), Data: props,
					})
					nodeSeen[name] = struct{}{}
				}
			}
			if rels, ok := record.Values[1].([]interface{}); ok {
				for _, relVal := range rels {
					if rel, ok := relVal.(neo4j.Relationship); ok {
						edgeKey := fmt.Sprintf("%d", rel.Id)
						edges = append(edges, model.G6Edge{
							ID:     edgeKey,
							Source: fmt.Sprintf("%d", rel.StartId),
							Target: fmt.Sprintf("%d", rel.EndId),
							Label:  rel.Type,
							Status: asString(rel.Props["status"]),
							Reason: asString(rel.Props["reason"]),
							Data:   rel.Props,
						})
					}
				}
			}
		}
		return nil, neighborResult.Err()
	})
	if err != nil {
		return model.G6GraphResponse{}, err
	}

	return model.G6GraphResponse{Nodes: nodes, Edges: edges}, nil
}

// ==================== 文件管理 ====================

func (r *graphRepository) CreateFile(ctx context.Context, userID, name, fileGroupID string) (string, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	fileID := fmt.Sprintf("file_%d", time.Now().UnixNano())
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		params := map[string]interface{}{
			"user_id":       userID,
			"file_id":       fileID,
			"name":          name,
			"file_group_id": fileGroupID,
			"created_at":    time.Now().UTC().Format(time.RFC3339),
			"updated_at":    time.Now().UTC().Format(time.RFC3339),
		}
		_, err := tx.Run(ctx, `
			CREATE (f:File {user_id: $user_id, file_id: $file_id, name: $name,
			       file_group_id: $file_group_id, created_at: $created_at, updated_at: $updated_at})
		`, params)
		return nil, err
	})
	return fileID, err
}

func (r *graphRepository) CreateFileGroup(ctx context.Context, userID, name string) (string, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	groupID := fmt.Sprintf("group_%d", time.Now().UnixNano())
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, `
			CREATE (fg:FileGroup {user_id: $user_id, group_id: $group_id, name: $name,
			       file_ids: [], created_at: $created_at})
		`, map[string]interface{}{
			"user_id":    userID,
			"group_id":   groupID,
			"name":       name,
			"created_at": time.Now().UTC().Format(time.RFC3339),
		})
		return nil, err
	})
	return groupID, err
}

func (r *graphRepository) ListUserFiles(ctx context.Context, userID string) ([]model.UserFile, []model.FileGroup, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	files := make([]model.UserFile, 0)
	groups := make([]model.FileGroup, 0)

	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// 查询文件
		fileResult, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id})
			RETURN f ORDER BY coalesce(f.pinned, false) DESC, f.created_at DESC
		`, map[string]interface{}{"user_id": userID})
		if err != nil {
			return nil, err
		}
		for fileResult.Next(ctx) {
			node, _ := fileResult.Record().Get("f")
			if f, ok := node.(neo4j.Node); ok {
				files = append(files, model.UserFile{
					ID:          asString(f.Props["file_id"]),
					Name:        asString(f.Props["name"]),
					UserID:      userID,
					FileGroupID: asString(f.Props["file_group_id"]),
					Pinned:      f.Props["pinned"] == true,
					CreatedAt:   asString(f.Props["created_at"]),
					UpdatedAt:   asString(f.Props["updated_at"]),
				})
			}
		}

		// 查询文件组
		groupResult, err := tx.Run(ctx, `
			MATCH (fg:FileGroup {user_id: $user_id})
			RETURN fg ORDER BY coalesce(fg.pinned, false) DESC, fg.created_at DESC
		`, map[string]interface{}{"user_id": userID})
		if err != nil {
			return nil, err
		}
		for groupResult.Next(ctx) {
			node, _ := groupResult.Record().Get("fg")
			if g, ok := node.(neo4j.Node); ok {
				fileIDs := make([]string, 0)
				if ids, ok := g.Props["file_ids"].([]interface{}); ok {
					for _, id := range ids {
						fileIDs = append(fileIDs, fmt.Sprintf("%v", id))
					}
				}
				groups = append(groups, model.FileGroup{
					ID:      asString(g.Props["group_id"]),
					Name:    asString(g.Props["name"]),
					UserID:  userID,
					FileIDs: fileIDs,
					Pinned:  g.Props["pinned"] == true,
				})
			}
		}
		return nil, nil
	})

	return files, groups, err
}

func (r *graphRepository) DeleteFile(ctx context.Context, userID, fileID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// 删除文件节点及其关联的概念节点
		_, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id, file_id: $file_id}) DETACH DELETE f
		`, map[string]interface{}{"user_id": userID, "file_id": fileID})
		return nil, err
	})
	return err
}

func (r *graphRepository) DeleteFileGroup(ctx context.Context, userID, groupID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, `
			MATCH (fg:FileGroup {user_id: $user_id, group_id: $group_id}) DETACH DELETE fg
		`, map[string]interface{}{"user_id": userID, "group_id": groupID})
		return nil, err
	})
	return err
}

func (r *graphRepository) RenameFile(ctx context.Context, userID, fileID, newName string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id, file_id: $file_id})
			SET f.name = $name, f.updated_at = $now
		`, map[string]interface{}{"user_id": userID, "file_id": fileID, "name": newName, "now": time.Now().UTC().Format(time.RFC3339)})
		return nil, err
	})
	return err
}

func (r *graphRepository) RenameFileGroup(ctx context.Context, userID, groupID, newName string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, `
			MATCH (fg:FileGroup {user_id: $user_id, group_id: $group_id})
			SET fg.name = $name
		`, map[string]interface{}{"user_id": userID, "group_id": groupID, "name": newName})
		return nil, err
	})
	return err
}

func (r *graphRepository) TogglePinFile(ctx context.Context, userID, fileID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id, file_id: $file_id})
			SET f.pinned = NOT coalesce(f.pinned, false)
		`, map[string]interface{}{"user_id": userID, "file_id": fileID})
		return nil, err
	})
	return err
}

func (r *graphRepository) TogglePinFileGroup(ctx context.Context, userID, groupID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, `
			MATCH (fg:FileGroup {user_id: $user_id, group_id: $group_id})
			SET fg.pinned = NOT coalesce(fg.pinned, false)
		`, map[string]interface{}{"user_id": userID, "group_id": groupID})
		return nil, err
	})
	return err
}

func (r *graphRepository) AddFileToGroup(ctx context.Context, userID, fileID, groupID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id, file_id: $file_id})
			SET f.file_group_id = $group_id
			WITH f
			MATCH (fg:FileGroup {user_id: $user_id, group_id: $group_id})
			SET fg.file_ids = CASE WHEN $file_id IN fg.file_ids THEN fg.file_ids ELSE fg.file_ids + $file_id END
		`, map[string]interface{}{"user_id": userID, "file_id": fileID, "group_id": groupID})
		return nil, err
	})
	return err
}

// ==================== 对话管理 ====================

func (r *graphRepository) GetOrCreateConversation(ctx context.Context, userID, fileID, fileGroupID string) (model.Conversation, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	var conv model.Conversation
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// 查找已有对话（按 file_id 或 file_group_id 匹配）
		convID := fmt.Sprintf("conv_%d", time.Now().UnixNano())
		query := `
			MATCH (c:Conversation {user_id: $user_id})
			WHERE ($file_id <> '' AND c.file_id = $file_id)
			   OR ($file_group_id <> '' AND c.file_group_id = $file_group_id)
			RETURN c ORDER BY c.updated_at DESC LIMIT 1
		`
		result, err := tx.Run(ctx, query, map[string]interface{}{
			"user_id":        userID,
			"file_id":        fileID,
			"file_group_id":  fileGroupID,
		})
		if err != nil {
			return nil, err
		}

		if result.Next(ctx) {
			node, _ := result.Record().Get("c")
			if n, ok := node.(neo4j.Node); ok {
				convID = asString(n.Props["conversation_id"])
			}
		} else {
			// 无已有对话，创建新对话节点
			title := "新对话"
			if fileID != "" {
				title = "文件对话"
			} else if fileGroupID != "" {
				title = "文件组对话"
			}
			_, err := tx.Run(ctx, `
				CREATE (c:Conversation {
					conversation_id: $conv_id, user_id: $user_id,
					file_id: $file_id, file_group_id: $file_group_id,
					title: $title, created_at: $now, updated_at: $now
				})
			`, map[string]interface{}{
				"conv_id":        convID,
				"user_id":        userID,
				"file_id":        fileID,
				"file_group_id":  fileGroupID,
				"title":          title,
				"now":            time.Now().UTC().Format(time.RFC3339),
			})
			if err != nil {
				return nil, err
			}
		}

		// 加载对话消息
		msgResult, err := tx.Run(ctx, `
			MATCH (c:Conversation {conversation_id: $conv_id})
			OPTIONAL MATCH (c)-[:CONTAINS]->(m:Message)
			RETURN c, m ORDER BY m.timestamp
		`, map[string]interface{}{"conv_id": convID})
		if err != nil {
			return nil, err
		}

		messages := make([]model.ConversationMessage, 0)
		for msgResult.Next(ctx) {
			record := msgResult.Record()
			// 提取对话节点信息
			if convNode, ok := record.Values[0].(neo4j.Node); ok {
				conv.ID = asString(convNode.Props["conversation_id"])
				conv.FileID = asString(convNode.Props["file_id"])
				conv.FileGroupID = asString(convNode.Props["file_group_id"])
				conv.UserID = userID
				conv.Title = asString(convNode.Props["title"])
				conv.CreatedAt = asString(convNode.Props["created_at"])
				conv.UpdatedAt = asString(convNode.Props["updated_at"])
			}
			// 提取消息
			if msgNode, ok := record.Values[1].(neo4j.Node); ok {
				messages = append(messages, model.ConversationMessage{
					Role:      asString(msgNode.Props["role"]),
					Content:   asString(msgNode.Props["content"]),
					Timestamp: asString(msgNode.Props["timestamp"]),
				})
			}
		}
		conv.Messages = messages
		if len(conv.Messages) == 0 {
			conv.Messages = []model.ConversationMessage{}
		}
		return nil, nil
	})

	return conv, err
}

func (r *graphRepository) SaveMessage(ctx context.Context, req model.SaveMessageRequest, userID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// 确保对话存在
	convID := req.ConversationID
	if convID == "" {
		var err error
		conv, err := r.GetOrCreateConversation(ctx, userID, req.FileID, req.FileGroupID)
		if err != nil {
			return err
		}
		convID = conv.ID
	}

	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, `
			MATCH (c:Conversation {conversation_id: $conv_id})
			CREATE (m:Message {message_id: $msg_id, role: $role, content: $content, timestamp: $now})
			CREATE (c)-[:CONTAINS]->(m)
			SET c.updated_at = $now
		`, map[string]interface{}{
			"conv_id": convID,
			"msg_id":  msgID,
			"role":    req.Role,
			"content": req.Content,
			"now":     now,
		})
		return nil, err
	})
	return err
}

func (r *graphRepository) GetConversation(ctx context.Context, userID, conversationID string) (model.Conversation, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	var conv model.Conversation
	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, `
			MATCH (c:Conversation {user_id: $user_id, conversation_id: $conv_id})
			OPTIONAL MATCH (c)-[:CONTAINS]->(m:Message)
			RETURN c, m ORDER BY m.timestamp
		`, map[string]interface{}{"user_id": userID, "conv_id": conversationID})
		if err != nil {
			return nil, err
		}

		messages := make([]model.ConversationMessage, 0)
		for result.Next(ctx) {
			record := result.Record()
			if convNode, ok := record.Values[0].(neo4j.Node); ok && conv.ID == "" {
				conv.ID = asString(convNode.Props["conversation_id"])
				conv.FileID = asString(convNode.Props["file_id"])
				conv.FileGroupID = asString(convNode.Props["file_group_id"])
				conv.UserID = userID
				conv.Title = asString(convNode.Props["title"])
				conv.CreatedAt = asString(convNode.Props["created_at"])
				conv.UpdatedAt = asString(convNode.Props["updated_at"])
			}
			if msgNode, ok := record.Values[1].(neo4j.Node); ok {
				messages = append(messages, model.ConversationMessage{
					Role:      asString(msgNode.Props["role"]),
					Content:   asString(msgNode.Props["content"]),
					Timestamp: asString(msgNode.Props["timestamp"]),
				})
			}
		}
		conv.Messages = messages
		if len(conv.Messages) == 0 {
			conv.Messages = []model.ConversationMessage{}
		}
		return nil, nil
	})
	return conv, err
}

func (r *graphRepository) DeleteConversation(ctx context.Context, userID, conversationID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, `
			MATCH (c:Conversation {user_id: $user_id, conversation_id: $conv_id})
			OPTIONAL MATCH (c)-[:CONTAINS]->(m:Message)
			DETACH DELETE c, m
		`, map[string]interface{}{"user_id": userID, "conv_id": conversationID})
		return nil, err
	})
	return err
}

// ==================== 辅助函数 ====================

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

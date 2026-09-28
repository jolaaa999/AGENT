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
	CreateFileWithContent(ctx context.Context, userID, name, content, fileGroupID string) (string, error)
	UpdateFileContent(ctx context.Context, userID, fileID, content string) error
	GetFilesMarkdown(ctx context.Context, userID, fileID, fileGroupID string) (string, error)
	RebuildMarkdownFromGraph(ctx context.Context, userID, fileID string) (string, error)
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
			// 注意：这里**不能**写 t.status / t.reason。
			// 边也会带 status（如 AI 补全关系是 supplement），若顺手 SET 到目标节点，
			// 就会把节点自己由事实校验得出的 error 状态覆盖掉，
			// 表现为「诊断说有 5 个错误节点，但图上全是 supplement」。
			// 节点状态只由上面的实体循环写入，这里只补 last_relation。
			query := fmt.Sprintf(`
				MERGE (s:Concept {user_id: $user_id, name: $source})
				ON CREATE SET s.type = "Concept"
				MERGE (t:Concept {user_id: $user_id, name: $target})
				ON CREATE SET t.type = "Concept"
				SET t.last_relation = $relation_desc,
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

// maxPathDepthLimit 是逆向学习路径允许的最大回溯层数。
//
// 这是前后端共同契约：frontend/src/view/main.vue 的 MAX_PATH_DEPTH_LIMIT
// 必须等于这个值。两边不一致时，用户会在界面上选到一个注定被后端
// 截断的深度，而当时的实现连提示都没有（Meta 从未回传）。
const maxPathDepthLimit = 6

// clampPathDepth 把用户请求的深度夹到 [1, maxPathDepthLimit]。
//
// 第二个返回值表示「是否因为超过上限而被夹」——仅在下调时才算截断。
// 小于 1 的输入向上抬到 1，这属于修正非法值，不算「截断」，
// 因此不触发「深度已按上限计算」的提示（否则用户填 0 会收到一句驴唇不对马嘴的警告）。
func clampPathDepth(maxDepth int) (applied int, clamped bool) {
	if maxDepth < 1 {
		return 1, false
	}
	if maxDepth > maxPathDepthLimit {
		return maxPathDepthLimit, true
	}
	return maxDepth, false
}

func (r *graphRepository) GetPathsToConcept(ctx context.Context, userID, concept string, maxDepth int) (model.PathResponse, error) {
	// 记录用户请求的原始深度，再夹到后端上限。
	// 两者都要回传给前端：用户设了 12 结果只按 6 算的时候，
	// 必须明确告知「深度已按上限 6 计算」，不能静默降级。
	requestedDepth := maxDepth
	maxDepth, depthClamped := clampPathDepth(maxDepth)

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
				// 逆向依赖树只沿 PREREQUISITE_OF 追溯（见上面 reverseQuery 的
				// `WHERE all(rel IN r WHERE type(rel) = 'PREREQUISITE_OF')`），
				// 所以这里的每一条都必然是「真正的学习前置依赖」。
				//
				// 之前这两个字段没被赋值，导致前端 `depTree.filter(n => n.strength === "strong")`
				// 恒为 0，状态栏一直显示「0 个前置依赖」——明明图上画着 5 个前置节点。
				Strength: "strong",
				Via:      "PREREQUISITE_OF",
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
				if src == "" {
					src = fmt.Sprintf("%d", rel.StartId)
				}
				if tgt == "" {
					tgt = fmt.Sprintf("%d", rel.EndId)
				}
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

	// 统计强弱依赖，供前端如实展示。
	// 说明：逆向依赖树只走 PREREQUISITE_OF，理论上全部是 strong；
	// 这里仍然按实际字段统计（而不是直接写死 len），这样以后若补上
	// weak 兜底逻辑，统计会自动跟着变准。
	strongCount, weakCount := 0, 0
	for _, n := range dependencyTree {
		switch n.Strength {
		case "strong":
			strongCount++
		case "weak":
			weakCount++
		}
	}

	// Meta 必须回传：前端会用它显示「深度已按上限计算」的提示。
	// 此前该字段从未被赋值（omitempty + nil → 前端拿到 undefined），
	// 于是当用户把深度设成 7~12（前端上限 12）时，后端静默按 6 计算，
	// 用户完全不知道自己的设置被截断了。
	meta := &model.PathQueryMeta{
		RequestedDepth: requestedDepth,
		AppliedDepth:   maxDepth,
		DepthClamped:   depthClamped,
		MaxDepthLimit:  maxPathDepthLimit,
		StrongCount:    strongCount,
		WeakCount:      weakCount,
		RelatedCount:   len(nodeList),
	}

	return model.PathResponse{
		Concept:        concept,
		Paths:          []model.G6GraphResponse{*allRelated},
		DependencyTree: dependencyTree,
		AllRelated:     allRelated,
		Meta:           meta,
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
	// 边去重：多跳查询会让同一条边在多条路径中重复出现
	edgeSeen := make(map[string]struct{})

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
		//
		// 关键：边的 source/target 必须返回「概念名」，与节点 ID 保持一致。
		// 若用 rel.StartId / rel.EndId（Neo4j 内部数字 ID），前端拿到的边会指向
		// 形如 "278" 的不存在节点，G6 写坐标时抛 "Node not found for id: 348"，
		// 表现为「展开失败」。这里直接从关系端点取 name。
		query := fmt.Sprintf(`
			MATCH (center:Concept {user_id: $user_id, name: $node_id})-[r*1..%d]-(neighbor:Concept {user_id: $user_id})
			RETURN neighbor, r AS relations,
			       [rel IN r | startNode(rel).name] AS source_names,
			       [rel IN r | endNode(rel).name] AS target_names
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
			rels, ok := record.Values[1].([]interface{})
			if !ok {
				continue
			}
			sourceNames := toStringSlice(record.Values[2])
			targetNames := toStringSlice(record.Values[3])
			for idx, relVal := range rels {
				rel, ok := relVal.(neo4j.Relationship)
				if !ok {
					continue
				}
				if idx >= len(sourceNames) || idx >= len(targetNames) {
					continue
				}
				source := sourceNames[idx]
				target := targetNames[idx]
				if source == "" || target == "" {
					continue
				}
				edgeKey := fmt.Sprintf("%s-%s-%s", source, rel.Type, target)
				if _, exists := edgeSeen[edgeKey]; exists {
					continue
				}
				edgeSeen[edgeKey] = struct{}{}
				edges = append(edges, model.G6Edge{
					ID:     edgeKey,
					Source: source,
					Target: target,
					Label:  rel.Type,
					Status: asString(rel.Props["status"]),
					Reason: asString(rel.Props["reason"]),
					Data:   rel.Props,
				})
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
		// 必须在同一个事务里级联删除该文件产生的概念节点。
		//
		// 原实现只 DETACH DELETE 了 File 节点，注释虽写着「及其关联的概念节点」，
		// 但 Cypher 里并没有删 Concept，导致：删掉文件后，它抽取出的概念节点
		// 仍带着已失效的 file_id 留在图里，全图查询仍会返回它们——
		// 用户「删了文件但图谱里概念还在」，且永远无法通过界面清除。
		//
		// 用 file_id 精确匹配（概念节点写图时都会带上 file_id），
		// 再 DETACH DELETE 以一并清掉挂在它们上面的关系。
		//
		// 先 count 再删：Neo4j 不允许在 DETACH DELETE 之后 RETURN 被删的变量，
		// 分开写才能既判断命中数又完成删除。
		check, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id, file_id: $file_id})
			RETURN count(f) AS matched
		`, map[string]interface{}{"user_id": userID, "file_id": fileID})
		if err != nil {
			return nil, err
		}
		if err := countMatchedError(ctx, check, "file", fileID); err != nil {
			return nil, err
		}
		if _, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id, file_id: $file_id})
			DETACH DELETE f
		`, map[string]interface{}{"user_id": userID, "file_id": fileID}); err != nil {
			return nil, err
		}
		if _, err := tx.Run(ctx, `
			MATCH (n:Concept {user_id: $user_id, file_id: $file_id})
			DETACH DELETE n
		`, map[string]interface{}{"user_id": userID, "file_id": fileID}); err != nil {
			return nil, err
		}
		// 同步把该文件从所属文件组的 file_ids 里摘掉，
		// 避免组里留下指向已删文件的悬空 id。
		if _, err := tx.Run(ctx, `
			MATCH (fg:FileGroup {user_id: $user_id})
			WHERE $file_id IN fg.file_ids
			SET fg.file_ids = [x IN fg.file_ids WHERE x <> $file_id]
		`, map[string]interface{}{"user_id": userID, "file_id": fileID}); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return err
}

func (r *graphRepository) DeleteFileGroup(ctx context.Context, userID, groupID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// 先 count 再删（DETACH DELETE 后不能 RETURN 被删变量）
		check, err := tx.Run(ctx, `
			MATCH (fg:FileGroup {user_id: $user_id, group_id: $group_id})
			RETURN count(fg) AS matched
		`, map[string]interface{}{"user_id": userID, "group_id": groupID})
		if err != nil {
			return nil, err
		}
		// 不存在的 group_id 必须报错，否则接口回 "deleted" 但什么都没删
		if err := countMatchedError(ctx, check, "file group", groupID); err != nil {
			return nil, err
		}
		if _, err := tx.Run(ctx, `
			MATCH (fg:FileGroup {user_id: $user_id, group_id: $group_id})
			DETACH DELETE fg
		`, map[string]interface{}{"user_id": userID, "group_id": groupID}); err != nil {
			return nil, err
		}
		return nil, nil
	})
	return err
}

func (r *graphRepository) RenameFile(ctx context.Context, userID, fileID, newName string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id, file_id: $file_id})
			SET f.name = $name, f.updated_at = $now
			RETURN count(f) AS matched
		`, map[string]interface{}{"user_id": userID, "file_id": fileID, "name": newName, "now": time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			return nil, err
		}
		// 不存在的 file_id 必须报错，否则接口回 "renamed" 但什么都没改
		return nil, countMatchedError(ctx, res, "file", fileID)
	})
	return err
}

func (r *graphRepository) RenameFileGroup(ctx context.Context, userID, groupID, newName string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, `
			MATCH (fg:FileGroup {user_id: $user_id, group_id: $group_id})
			SET fg.name = $name
			RETURN count(fg) AS matched
		`, map[string]interface{}{"user_id": userID, "group_id": groupID, "name": newName})
		if err != nil {
			return nil, err
		}
		return nil, countMatchedError(ctx, res, "file group", groupID)
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
		// 必须**先**确认文件和目标文件组都存在，再修改任何数据。
		//
		// 原实现把两个操作写在一个 Cypher 里：
		//   MATCH (f:File) SET f.file_group_id = $group_id
		//   WITH f MATCH (fg:FileGroup) SET fg.file_ids = ...
		// 当 group_id 不存在时，第一段 SET 已经执行，第二段 MATCH 匹配 0 行
		// 不会报错（Cypher 里 MATCH 无命中不是错误）。结果是：
		//   接口返回 "added"，但文件被挂到了一个不存在的组上（悬空引用），
		//   该文件随后在按组筛选的视图里彻底消失。
		// 先做存在性校验，才能保证「要么都改，要么都不改」。
		check, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id, file_id: $file_id})
			RETURN count(f) AS matched
		`, map[string]interface{}{"user_id": userID, "file_id": fileID})
		if err != nil {
			return nil, err
		}
		if err := countMatchedError(ctx, check, "file", fileID); err != nil {
			return nil, err
		}

		checkGroup, err := tx.Run(ctx, `
			MATCH (fg:FileGroup {user_id: $user_id, group_id: $group_id})
			RETURN count(fg) AS matched
		`, map[string]interface{}{"user_id": userID, "group_id": groupID})
		if err != nil {
			return nil, err
		}
		if err := countMatchedError(ctx, checkGroup, "file group", groupID); err != nil {
			return nil, err
		}

		// 两者都存在，安全地执行关联
		if _, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id, file_id: $file_id})
			SET f.file_group_id = $group_id
			WITH f
			MATCH (fg:FileGroup {user_id: $user_id, group_id: $group_id})
			SET fg.file_ids = CASE WHEN $file_id IN fg.file_ids THEN fg.file_ids ELSE fg.file_ids + $file_id END
		`, map[string]interface{}{"user_id": userID, "file_id": fileID, "group_id": groupID}); err != nil {
			return nil, err
		}
		return nil, nil
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
			"user_id":       userID,
			"file_id":       fileID,
			"file_group_id": fileGroupID,
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
				"conv_id":       convID,
				"user_id":       userID,
				"file_id":       fileID,
				"file_group_id": fileGroupID,
				"title":         title,
				"now":           time.Now().UTC().Format(time.RFC3339),
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

func (r *graphRepository) CreateFileWithContent(ctx context.Context, userID, name, fileGroupID, content string) (string, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	fileID := fmt.Sprintf("file_%d", time.Now().UnixNano())
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, `
			CREATE (f:File {user_id: $user_id, file_id: $file_id, name: $name,
			       file_group_id: $file_group_id, content: $content,
			       created_at: $now, updated_at: $now})
		`, map[string]interface{}{
			"user_id":       userID,
			"file_id":       fileID,
			"name":          name,
			"file_group_id": fileGroupID,
			"content":       content,
			"now":           now,
		})
		return nil, err
	})
	return fileID, err
}

func (r *graphRepository) UpdateFileContent(ctx context.Context, userID, fileID, content string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id, file_id: $file_id})
			SET f.content = $content, f.updated_at = $now
			RETURN count(f) AS matched
		`, map[string]interface{}{
			"user_id": userID,
			"file_id": fileID,
			"content": content,
			"now":     time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		// 没有匹配到文件时必须显式报错，避免静默成功。
		//
		// 原实现调用 res.Consume(ctx)，但 Consume 只在「查询本身执行失败」时返回
		// 错误，MATCH 命中 0 行它是成功的——所以更新一个不存在的 file_id
		// 也会返回 nil，接口回 "saved"、前端提示「已保存到该文件」，
		// 而实际上一个字都没写进去（AI 改文档的静默丢失路径）。
		//
		// 改为读回 RETURN count(f) 的聚合值来判断是否真的命中了节点。
		if res.Next(ctx) {
			matched, _ := res.Record().Get("matched")
			if n, ok := matched.(int64); ok && n == 0 {
				return nil, fmt.Errorf("file not found: %s", fileID)
			}
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (r *graphRepository) GetFilesMarkdown(ctx context.Context, userID, fileID, fileGroupID string) (string, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id})
			WHERE ($file_id <> '' AND f.file_id = $file_id)
			   OR ($file_id = '' AND $file_group_id <> '' AND f.file_group_id = $file_group_id)
			RETURN f.name AS name, f.content AS content, f.created_at AS created_at
			ORDER BY f.created_at
		`, map[string]interface{}{
			"user_id":       userID,
			"file_id":       fileID,
			"file_group_id": fileGroupID,
		})
		if err != nil {
			return nil, err
		}

		var b strings.Builder
		for res.Next(ctx) {
			rec := res.Record()
			name := asString(rec.Values[0])
			content := asString(rec.Values[1])
			if strings.TrimSpace(content) == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n\n---\n\n")
			}
			if name != "" {
				b.WriteString("## " + name + "\n\n")
			}
			b.WriteString(content)
		}
		return b.String(), res.Err()
	})
	if err != nil {
		return "", err
	}
	md, _ := result.(string)
	return md, nil
}

func (r *graphRepository) RebuildMarkdownFromGraph(ctx context.Context, userID, fileID string) (string, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	type conceptRow struct {
		Name       string
		Definition string
		Status     string
		Reason     string
	}

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// 找文件名
		fileName := ""
		nameRes, err := tx.Run(ctx, `
			MATCH (f:File {user_id: $user_id, file_id: $file_id})
			RETURN f.name AS name LIMIT 1
		`, map[string]interface{}{"user_id": userID, "file_id": fileID})
		if err != nil {
			return nil, err
		}
		if nameRes.Next(ctx) {
			fileName = asString(nameRes.Record().Values[0])
		}

		// 取该文件下的概念。file_id 可能存在于节点属性上（规范字段），
		// 也用 BELONGS_TO 关系兜底，兼容历史数据写法不一致的情况。
		res, err := tx.Run(ctx, `
			MATCH (c:Concept {user_id: $user_id})
			WHERE c.file_id = $file_id
			   OR EXISTS {
			        MATCH (c)-[:BELONGS_TO]->(f:File {user_id: $user_id, file_id: $file_id})
			      }
			RETURN DISTINCT c.name AS name,
			       coalesce(c.definition, '') AS definition,
			       coalesce(c.status, '') AS status,
			       coalesce(c.reason, '') AS reason
			ORDER BY name
		`, map[string]interface{}{"user_id": userID, "file_id": fileID})
		if err != nil {
			return nil, err
		}

		rows := make([]conceptRow, 0)
		for res.Next(ctx) {
			rec := res.Record()
			rows = append(rows, conceptRow{
				Name:       asString(rec.Values[0]),
				Definition: asString(rec.Values[1]),
				Status:     asString(rec.Values[2]),
				Reason:     asString(rec.Values[3]),
			})
		}
		if err := res.Err(); err != nil {
			return nil, err
		}
		return map[string]interface{}{"name": fileName, "rows": rows}, nil
	})
	if err != nil {
		return "", err
	}

	payload, _ := result.(map[string]interface{})
	rows, _ := payload["rows"].([]conceptRow)
	fileName, _ := payload["name"].(string)

	if len(rows) == 0 {
		return "", nil
	}

	statusLabel := map[string]string{
		"correct":    "已掌握",
		"error":      "有错误",
		"supplement": "待补全",
	}

	var b strings.Builder
	title := fileName
	if strings.TrimSpace(title) == "" {
		title = fileID
	}
	b.WriteString("# " + title + "\n\n")
	b.WriteString("> 本文由知识图谱中的概念重建（原上传文件的正文未保存在云端）。\n")
	b.WriteString("> 内容结构可能与原始笔记不同，请核对后再使用。\n\n")

	for _, row := range rows {
		b.WriteString("### " + row.Name + "\n\n")
		if strings.TrimSpace(row.Definition) != "" {
			b.WriteString("- **定义**：" + row.Definition + "\n")
		}
		if strings.TrimSpace(row.Status) != "" {
			label := statusLabel[row.Status]
			if label == "" {
				label = row.Status
			}
			b.WriteString("- **状态**：" + label + "\n")
		}
		if strings.TrimSpace(row.Reason) != "" {
			b.WriteString("- **说明**：" + row.Reason + "\n")
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

func visitedKeys(visited map[string]struct{}) []string {
	keys := make([]string, 0, len(visited))
	for k := range visited {
		keys = append(keys, k)
	}
	return keys
}

func mustGet(record *neo4j.Record, key string) interface{} {
	value, _ := record.Get(key)
	return value
}

func fetchConceptNode(ctx context.Context, tx neo4j.ManagedTransaction, userID, name string) *model.G6Node {
	result, err := tx.Run(ctx, `
		MATCH (c:Concept {user_id: $user_id, name: $name})
		RETURN c LIMIT 1
	`, map[string]interface{}{"user_id": userID, "name": name})
	if err != nil {
		return nil
	}
	if !result.Next(ctx) {
		return nil
	}
	nodeVal, _ := result.Record().Get("c")
	node, ok := nodeVal.(neo4j.Node)
	if !ok {
		return nil
	}
	return &model.G6Node{
		ID:     name,
		Label:  name,
		Type:   asString(node.Props["type"]),
		Status: asString(node.Props["status"]),
		Reason: asString(node.Props["reason"]),
		Data:   node.Props,
	}
}

func toStringSlice(value interface{}) []string {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
			continue
		}
		out = append(out, "")
	}
	return out
}

// countMatchedError 读取查询里 RETURN count(x) AS matched 的聚合结果，
// 命中 0 个节点时返回 not found 错误。
//
// 背景：Neo4j 的 Result.Consume 只在「查询执行失败」时报错，
// MATCH 命中 0 行它是成功的。因此仅靠 Consume 无法区分
// 「成功修改了 1 个节点」和「压根没这个节点」，
// 会让删除/重命名/更新接口对不存在的资源也返回成功。
func countMatchedError(ctx context.Context, res neo4j.ResultWithContext, kind, id string) error {
	if res.Next(ctx) {
		matched, _ := res.Record().Get("matched")
		if n, ok := matched.(int64); ok && n == 0 {
			return fmt.Errorf("%s not found: %s", kind, id)
		}
	}
	return res.Err()
}

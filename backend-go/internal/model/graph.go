package model

// ==================== 请求/响应模型 ====================

type UploadNoteRequest struct {
	Markdown    string `json:"markdown" binding:"required"`
	UserID      string `json:"user_id"`
	FileID      string `json:"file_id"`       // 所属独立文件 ID
	FileGroupID string `json:"file_group_id"` // 所属文件组 ID（组内图谱融合）
	UseLangChain bool  `json:"use_langchain"` // 是否使用 LangChain 诊断流水线
}

type ParseRequest struct {
	Markdown string `json:"markdown"`
}

type ParseRelation struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
}

type ParseResponse struct {
	Chunks     []string        `json:"chunks"`
	Relations  []ParseRelation `json:"relations"`
	RetriesUse int             `json:"retries_used"`
}

// ==================== LangChain 诊断响应模型 ====================

type LangChainDiagnoseRequest struct {
	Markdown string `json:"markdown"`
}

type LangChainDiagnoseNode struct {
	Name       string `json:"name"`
	Definition string `json:"definition"`
	EntityType string `json:"entity_type"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
	Source     string `json:"source"` // 来源 Agent: ner / fact_check / supplement
}

type LangChainDiagnoseEdge struct {
	Source      string `json:"source"`
	Target      string `json:"target"`
	Relation    string `json:"relation"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	SourceAgent string `json:"source_agent,omitempty"`
}

type LangChainDiagnoseResponse struct {
	Success      bool                    `json:"success"`
	Nodes        []LangChainDiagnoseNode `json:"nodes"`
	Edges        []LangChainDiagnoseEdge `json:"edges"`
	Summary      string                  `json:"summary"`
	RetriesUsed  int                     `json:"retries_used"`
	ErrorMessage string                  `json:"error_message"`
}

// ==================== 图谱数据模型 ====================

type Entity struct {
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Status     string                 `json:"status"`
	Reason     string                 `json:"reason"`
	FileID     string                 `json:"file_id,omitempty"`
	FileGroupID string                `json:"file_group_id,omitempty"`
	Properties map[string]interface{} `json:"properties"`
}

type Relation struct {
	Source      string                 `json:"source"`
	Target      string                 `json:"target"`
	Type        string                 `json:"type"`
	Description string                 `json:"description"`
	Status      string                 `json:"status"`
	Reason      string                 `json:"reason"`
	Properties  map[string]interface{} `json:"properties"`
}

type GraphData struct {
	Entities  []Entity   `json:"entities"`
	Relations []Relation `json:"relations"`
}

// ==================== 文件管理模型 ====================

type FileGroup struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	UserID   string   `json:"user_id"`
	FileIDs  []string `json:"file_ids"` // 组内文件的 ID 列表
}

type UserFile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	UserID      string `json:"user_id"`
	FileGroupID string `json:"file_group_id,omitempty"` // 所属文件组（空=独立文件）
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ==================== G6 前端可视化模型 ====================

type G6Node struct {
	ID     string                 `json:"id"`
	Label  string                 `json:"label"`
	Type   string                 `json:"type"`
	Status string                 `json:"status,omitempty"`
	Reason string                 `json:"reason,omitempty"`
	FileID string                 `json:"file_id,omitempty"`
	Data   map[string]interface{} `json:"data,omitempty"`
}

type G6Edge struct {
	ID     string                 `json:"id"`
	Source string                 `json:"source"`
	Target string                 `json:"target"`
	Label  string                 `json:"label"`
	Status string                 `json:"status,omitempty"`
	Reason string                 `json:"reason,omitempty"`
	Data   map[string]interface{} `json:"data,omitempty"`
}

type G6GraphResponse struct {
	Nodes []G6Node `json:"nodes"`
	Edges []G6Edge `json:"edges"`
}

// ==================== 路径导航模型 ====================

// 依赖树节点（DFS 逆向查询结果）
type DependencyNode struct {
	Name   string `json:"name"`
	Depth  int    `json:"depth"`  // 距离目标概念的跳数
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type PathResponse struct {
	Concept        string           `json:"concept"`
	Paths          []G6GraphResponse `json:"paths"`
	DependencyTree []DependencyNode  `json:"dependency_tree,omitempty"` // 逆向技能树
	AllRelated     *G6GraphResponse  `json:"all_related,omitempty"`     // 所有相关节点（用于专注模式）
}

// ==================== 讲解模型 ====================

type ExplainRequest struct {
	Concept  string `json:"concept" binding:"required"`
	Markdown string `json:"markdown" binding:"required"`
	UserID   string `json:"user_id"`
}

type ExplainResponse struct {
	Concept     string `json:"concept"`
	Explanation string `json:"explanation"`
}

// ==================== AI 对话模型 ====================

type ChatRequest struct {
	UserMessage        string `json:"user_message" binding:"required"`
	GraphNodes         string `json:"graph_nodes"`
	GraphEdges         string `json:"graph_edges"`
	ConversationHistory string `json:"conversation_history"`
}

type ChatResponse struct {
	Reply          string `json:"reply"`
	ConversationID string `json:"conversation_id"`
}

// ==================== 学习路径指导模型 ====================

type LearningPathRequest struct {
	TargetConcept       string `json:"target_concept" binding:"required"`
	DependencyTreeJSON  string `json:"dependency_tree_json"`
	GraphNodesJSON      string `json:"graph_nodes_json"`
}

type LearningPathResponse struct {
	Guidance string `json:"guidance"`
}

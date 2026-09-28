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

type UserFile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	UserID      string `json:"user_id"`
	FileGroupID string `json:"file_group_id,omitempty"`
	Pinned      bool   `json:"pinned"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type FileGroup struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	UserID  string   `json:"user_id"`
	FileIDs []string `json:"file_ids"` // 组内文件的 ID 列表
	Pinned  bool     `json:"pinned"`
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
	// Strength 标识这条依赖的强度：
	//   "strong" = 沿 PREREQUISITE_OF 得到，是真正的学习前置依赖，先后顺序可信
	//   "weak"   = 仅靠 RELATED_TO / SUPPLEMENTS 等关联边兜底找到，只是「相关」，
	//              不代表应当先学它，前端需要分开呈现避免误导
	Strength string `json:"strength,omitempty"`
	// Via 记录命中所用的关系类型，便于在界面上解释「为什么它会被找到」
	Via string `json:"via,omitempty"`
}

type PathResponse struct {
	Concept        string            `json:"concept"`
	Paths          []G6GraphResponse `json:"paths"`
	DependencyTree []DependencyNode  `json:"dependency_tree,omitempty"` // 逆向技能树
	AllRelated     *G6GraphResponse  `json:"all_related,omitempty"`     // 所有相关节点（用于专注模式）
	// Meta 描述本次查询的执行情况（深度是否被截断、强/弱关联各多少），
	// 供前端如实告知用户，避免「设了 50 却按 12 算」这种静默降级。
	Meta *PathQueryMeta `json:"meta,omitempty"`
}

// PathQueryMeta 路径查询的执行元信息。
type PathQueryMeta struct {
	RequestedDepth int  `json:"requested_depth"` // 用户请求的深度
	AppliedDepth   int  `json:"applied_depth"`   // 实际生效的深度
	DepthClamped   bool `json:"depth_clamped"`   // 是否因为上限被截断
	MaxDepthLimit  int  `json:"max_depth_limit"` // 当前上限值
	StrongCount    int  `json:"strong_count"`    // 强依赖（PREREQUISITE_OF）节点数
	WeakCount      int  `json:"weak_count"`      // 弱关联兜底节点数
	RelatedCount   int  `json:"related_count"`   // 直接关联节点数
	WeakTruncated  bool `json:"weak_truncated"`  // 弱关联是否因数量上限被裁剪
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
	UserMessage    string `json:"user_message" binding:"required"`
	ConversationID string `json:"conversation_id"`
	Markdown       string `json:"markdown"`
	GraphNodes     string `json:"graph_nodes"`
	GraphEdges     string `json:"graph_edges"`
	ImageBase64    string `json:"image_base64"`
	// UserID 由网关按登录态注入，用于定位该用户的 AI 工作区（沙箱）；
	// 不接受前端指定，避免越权访问他人工作区。
	UserID string `json:"user_id"`
}

type ChatResponse struct {
	Reply           string `json:"reply"`
	ConversationID  string `json:"conversation_id"`
	EditedMarkdown  string `json:"edited_markdown,omitempty"`
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

// ==================== 对话模型 ====================

// Conversation 代表一个文件/文件组的独立对话
type Conversation struct {
	ID           string            `json:"id"`
	FileID       string            `json:"file_id,omitempty"`
	FileGroupID  string            `json:"file_group_id,omitempty"`
	UserID       string            `json:"user_id"`
	Title        string            `json:"title"`
	Messages     []ConversationMessage `json:"messages"`
	CreatedAt    string            `json:"created_at"`
	UpdatedAt    string            `json:"updated_at"`
}

type ConversationMessage struct {
	Role      string `json:"role"` // "user" | "ai"
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

type SaveMessageRequest struct {
	ConversationID string `json:"conversation_id"`
	FileID         string `json:"file_id,omitempty"`
	FileGroupID    string `json:"file_group_id,omitempty"`
	Role           string `json:"role" binding:"required"`
	Content        string `json:"content" binding:"required"`
}

type CreateConversationRequest struct {
	FileID      string `json:"file_id,omitempty"`
	FileGroupID string `json:"file_group_id,omitempty"`
	Title       string `json:"title"`
}

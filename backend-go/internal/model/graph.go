package model

type UploadNoteRequest struct {
	Markdown string `json:"markdown" binding:"required"`
	UserID   string `json:"user_id"`
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

type Entity struct {
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	Status     string                 `json:"status"`
	Reason     string                 `json:"reason"`
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

type G6Node struct {
	ID     string                 `json:"id"`
	Label  string                 `json:"label"`
	Type   string                 `json:"type"`
	Status string                 `json:"status,omitempty"`
	Reason string                 `json:"reason,omitempty"`
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

type PathResponse struct {
	Concept string            `json:"concept"`
	Paths   []G6GraphResponse `json:"paths"`
}

type ExplainRequest struct {
	Concept  string `json:"concept" binding:"required"`
	Markdown string `json:"markdown" binding:"required"`
	UserID   string `json:"user_id"`
}

type ExplainResponse struct {
	Concept     string `json:"concept"`
	Explanation string `json:"explanation"`
}

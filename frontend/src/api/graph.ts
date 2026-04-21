export interface UploadNotePayload {
  markdown: string;
  user_id?: string;
}

export interface GraphNode {
  id: string;
  label: string;
  type?: string;
  status?: "correct" | "error" | "supplement" | string;
  //supplement 表示需要补充前置知识点
  reason?: string;
  data?: Record<string, unknown>;
}

export interface GraphEdge {
  id: string;
  source: string;
  target: string;
  label: string;
  status?: "correct" | "error" | "supplement" | string;
  reason?: string;
  confidence?: number;
  data?: Record<string, unknown>;
}

export interface GraphResponse {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface PathResponse {
  concept: string;
  paths: GraphResponse[];
}

export interface ExplainPayload {
  concept: string;
  markdown: string;
  user_id?: string;
}

export interface ExplainResponse {
  concept: string;
  explanation: string;
}

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers ?? {})
    },
    ...init
  });

  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Request failed with status ${response.status}`);
  }

  return (await response.json()) as T;
}

export function uploadNote(payload: UploadNotePayload) {
  return request<Record<string, unknown>>("/upload-note", {
    method: "POST",
    body: JSON.stringify(payload)
  });
}

export function getGraphAll(userId?: string) {
  const query = userId ? `?user_id=${encodeURIComponent(userId)}` : "";
  return request<GraphResponse>(`/graph/all${query}`);
}

export function getGraphPath(concept: string, userId?: string, maxDepth = 3) {
  const params = new URLSearchParams({
    concept,
    maxDepth: String(maxDepth)
  });
  if (userId) params.set("user_id", userId);
  return request<PathResponse>(`/graph/path?${params.toString()}`);
}

export function getNodeNeighbors(nodeId: string, userId?: string, depth = 1) {
  const params = new URLSearchParams({
    node_id: nodeId,
    depth: String(depth)
  });
  if (userId) params.set("user_id", userId);
  return request<GraphResponse>(`/graph/neighbors?${params.toString()}`);
}

export function explainConcept(payload: ExplainPayload) {
  return request<ExplainResponse>("/graph/explain", {
    method: "POST",
    body: JSON.stringify(payload)
  });
}

export interface UploadNotePayload {
  markdown: string;
  user_id?: string;
}

export interface GraphNode {
  id: string;
  label: string;
  type?: string;
  status?: "correct" | "error" | "supplement" | string;
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

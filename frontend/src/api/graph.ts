// ==================== 类型定义 ====================

export interface UploadNotePayload {
  markdown: string;
  user_id?: string;
  file_id?: string;
  file_group_id?: string;
  use_langchain?: boolean;
}

export interface GraphNode {
  id: string;
  label: string;
  type?: string;
  status?: "correct" | "error" | "supplement" | string;
  reason?: string;
  file_id?: string;
  data?: Record<string, unknown>;
}

export interface GraphEdge {
  id: string;
  source: string;
  target: string;
  label: string;
  confidence?: number;
  status?: "correct" | "error" | "supplement" | string;
  reason?: string;
  data?: Record<string, unknown>;
}

export interface GraphResponse {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface DependencyNode {
  name: string;
  depth: number;
  status?: string;
  reason?: string;
}

export interface PathResponse {
  concept: string;
  paths: GraphResponse[];
  dependency_tree?: DependencyNode[];
  all_related?: GraphResponse;
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

// ==================== 文件管理类型 ====================

export interface UserFile {
  id: string;
  name: string;
  user_id: string;
  file_group_id?: string;
  pinned?: boolean;
  created_at: string;
  updated_at: string;
}

export interface FileGroup {
  id: string;
  name: string;
  user_id: string;
  file_ids: string[];
  pinned?: boolean;
}

// ==================== AI 对话类型 ====================

export interface ChatRequest {
  user_message: string;
  conversation_id?: string;
  markdown?: string;
  graph_nodes?: string;
  graph_edges?: string;
  image_base64?: string;
}

export interface ChatResponse {
  reply: string;
  conversation_id: string;
  edited_markdown?: string;
}

export interface LearningPathRequest {
  target_concept: string;
  dependency_tree_json?: string;
  graph_nodes_json?: string;
}

export interface LearningPathResponse {
  guidance: string;
}

// ==================== API 客户端 ====================

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
    ...init,
  });

  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || `Request failed with status ${response.status}`);
  }

  return (await response.json()) as T;
}

// ==================== 图谱相关 ====================

export function uploadNote(payload: UploadNotePayload) {
  return request<Record<string, unknown>>("/upload-note", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

/** 使用 LangChain 诊断流水线上传笔记（NER -> FactCheck -> Supplement） */
export function uploadNoteLangChain(payload: UploadNotePayload) {
  return request<Record<string, unknown>>("/upload-note-langchain", {
    method: "POST",
    body: JSON.stringify({ ...payload, use_langchain: true }),
  });
}

export function getGraphAll(params?: { user_id?: string; file_id?: string; file_group_id?: string }) {
  const query = new URLSearchParams();
  if (params?.user_id) query.set("user_id", params.user_id);
  if (params?.file_id) query.set("file_id", params.file_id);
  if (params?.file_group_id) query.set("file_group_id", params.file_group_id);
  const qs = query.toString();
  return request<GraphResponse>(`/graph/all${qs ? "?" + qs : ""}`);
}

export function getGraphPath(concept: string, userId?: string, maxDepth = 3) {
  const params = new URLSearchParams({ concept, maxDepth: String(maxDepth) });
  if (userId) params.set("user_id", userId);
  return request<PathResponse>(`/graph/path?${params.toString()}`);
}

export function getNodeNeighbors(nodeId: string, userId?: string, depth = 1) {
  const params = new URLSearchParams({ node_id: nodeId, depth: String(depth) });
  if (userId) params.set("user_id", userId);
  return request<GraphResponse>(`/graph/neighbors?${params.toString()}`);
}

export function explainConcept(payload: ExplainPayload) {
  return request<ExplainResponse>("/graph/explain", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

// ==================== 文件管理 ====================

export function listUserFiles(userId?: string) {
  const query = userId ? `?user_id=${encodeURIComponent(userId)}` : "";
  return request<{ files: UserFile[]; file_groups: FileGroup[] }>(`/files${query}`);
}

export function createFile(name: string, fileGroupId?: string, userId?: string) {
  return request<{ file_id: string; name: string }>("/files/create", {
    method: "POST",
    body: JSON.stringify({ name, file_group_id: fileGroupId, user_id: userId }),
  });
}

export function createFileGroup(name: string, userId?: string) {
  return request<{ group_id: string; name: string }>("/files/group/create", {
    method: "POST",
    body: JSON.stringify({ name, user_id: userId }),
  });
}

export function deleteFile(fileId: string, userId?: string) {
  return request<{ status: string }>(`/files/delete?file_id=${encodeURIComponent(fileId)}&user_id=${userId ?? ""}`, {
    method: "DELETE",
  });
}

export function deleteFileGroup(groupId: string, userId?: string) {
  return request<{ status: string }>(`/files/group/delete?group_id=${encodeURIComponent(groupId)}&user_id=${userId ?? ""}`, {
    method: "DELETE",
  });
}

export function renameFile(fileId: string, newName: string, userId?: string) {
  return request<{ status: string }>("/files/rename", {
    method: "PUT",
    body: JSON.stringify({ file_id: fileId, new_name: newName, user_id: userId }),
  });
}

export function renameFileGroup(groupId: string, newName: string, userId?: string) {
  return request<{ status: string }>("/files/group/rename", {
    method: "PUT",
    body: JSON.stringify({ group_id: groupId, new_name: newName, user_id: userId }),
  });
}

export function addFileToGroup(fileId: string, groupId: string, userId?: string) {
  return request<{ status: string }>("/files/add-to-group", {
    method: "POST",
    body: JSON.stringify({ file_id: fileId, group_id: groupId, user_id: userId }),
  });
}

export function togglePinFile(fileId: string, userId?: string) {
  return request<{ status: string }>(`/files/pin?file_id=${encodeURIComponent(fileId)}&user_id=${userId ?? ""}`, {
    method: "PUT",
  });
}

export function togglePinFileGroup(groupId: string, userId?: string) {
  return request<{ status: string }>(`/files/group/pin?group_id=${encodeURIComponent(groupId)}&user_id=${userId ?? ""}`, {
    method: "PUT",
  });
}

// ==================== 对话管理 ====================

export interface ConversationMessage {
  role: "user" | "ai";
  content: string;
  timestamp?: string;
}

export interface Conversation {
  id: string;
  file_id?: string;
  file_group_id?: string;
  user_id: string;
  title: string;
  messages: ConversationMessage[];
  created_at: string;
  updated_at: string;
}

export function getConversation(params: { file_id?: string; file_group_id?: string; conversation_id?: string; user_id?: string }) {
  const query = new URLSearchParams();
  if (params.file_id) query.set("file_id", params.file_id);
  if (params.file_group_id) query.set("file_group_id", params.file_group_id);
  if (params.conversation_id) query.set("conversation_id", params.conversation_id);
  if (params.user_id) query.set("user_id", params.user_id);
  return request<Conversation>(`/conversation?${query.toString()}`);
}

export function saveMessage(payload: { conversation_id: string; file_id?: string; file_group_id?: string; role: string; content: string }) {
  return request<{ status: string }>("/conversation/message", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function deleteConversation(conversationId: string, userId?: string) {
  return request<{ status: string }>(`/conversation?conversation_id=${encodeURIComponent(conversationId)}&user_id=${userId ?? ""}`, {
    method: "DELETE",
  });
}

// ==================== AI 对话 ====================

export function chatWithContext(payload: ChatRequest) {
  return request<ChatResponse>("/graph/chat", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

export function getLearningPath(payload: LearningPathRequest) {
  return request<LearningPathResponse>("/graph/learning-path", {
    method: "POST",
    body: JSON.stringify(payload),
  });
}

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
  /**
   * 依赖强度：
   * - "strong" = 沿 PREREQUISITE_OF 得到，是真正的学习前置依赖，先后顺序可信
   * - "weak"   = 仅靠 RELATED_TO / SUPPLEMENTS 等关联边找到，只是「相关」
   */
  strength?: "strong" | "weak";
  /** 命中所用的关系类型 */
  via?: string;
}

/** 路径查询的执行元信息（深度是否截断、强/弱关联各多少） */
export interface PathQueryMeta {
  requested_depth: number;
  applied_depth: number;
  depth_clamped: boolean;
  max_depth_limit: number;
  strong_count: number;
  weak_count: number;
  related_count: number;
  weak_truncated: boolean;
}

export interface PathResponse {
  concept: string;
  paths: GraphResponse[];
  dependency_tree?: DependencyNode[];
  all_related?: GraphResponse;
  meta?: PathQueryMeta;
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

/**
 * 取回文件或整个文件组的 Markdown 正文。
 * 文件组对话需要它来聚合组内全部笔记作为 AI 上下文。
 */
export function getFilesMarkdown(params: { file_id?: string; file_group_id?: string; user_id?: string }) {
  const qs = new URLSearchParams();
  if (params.file_id) qs.set("file_id", params.file_id);
  if (params.file_group_id) qs.set("file_group_id", params.file_group_id);
  if (params.user_id) qs.set("user_id", params.user_id);
  return request<{ markdown: string; length: number }>(`/files/markdown?${qs.toString()}`);
}

/**
 * 把 docx / pdf / 图片 等文件上传给后端，转成 Markdown 文本。
 * 注意：这是 multipart/form-data，不能复用 JSON 的 request()（它会强制覆盖 Content-Type）。
 */
export async function extractToMarkdown(
  file: File,
  userId?: string,
): Promise<{ markdown: string; source_type: string; filename: string; original_name?: string }> {
  const form = new FormData();
  form.append("file", file);
  // 必须带上 user_id：后端要把原件存到该用户工作区的 original/ 目录下。
  // 不带的话会落到 default_user，用户就下载不到自己的原件了。
  if (userId) form.append("user_id", userId);
  const response = await fetch(`${API_BASE}/files/extract`, { method: "POST", body: form });
  const text = await response.text();
  let data: any = {};
  try {
    data = text ? JSON.parse(text) : {};
  } catch {
    data = { error: text };
  }
  if (!response.ok) {
    throw new Error(data.error || data.detail || `转换失败（HTTP ${response.status}）`);
  }
  return data;
}

/**
 * 把修改后的正文保存到指定文件节点（AI 编辑后落盘用）。
 */
export function updateFileContent(fileId: string, content: string, userId?: string) {
  return request<{ status: string }>("/files/content", {
    method: "PUT",
    body: JSON.stringify({ file_id: fileId, content, user_id: userId }),
  });
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

// ==================== AI 工作区（沙箱文件） ====================

export interface WorkspaceFile {
  name: string;
  size: number;
  mod_time: string;
  /** 是否为上传的原件（存于 original/ 子目录，字节级等于用户上传的文件） */
  is_original?: boolean;
  /** 展示类型：md / word / pdf / image / other */
  kind?: string;
}

export function listWorkspaceFiles(userId?: string) {
  return request<{ user_id: string; files: WorkspaceFile[] }>(
    `/workspace/files?user_id=${encodeURIComponent(userId ?? "")}`,
  );
}

export function readWorkspaceFile(path: string, userId?: string) {
  return request<{ path: string; content: string; length: number }>(
    `/workspace/file?path=${encodeURIComponent(path)}&user_id=${encodeURIComponent(userId ?? "")}`,
  );
}

export function writeWorkspaceFile(path: string, content: string, userId?: string) {
  return request<{ status: string; path: string }>("/workspace/file", {
    method: "PUT",
    body: JSON.stringify({ path, content, user_id: userId ?? "" }),
  });
}

/** 把某个已入库文件同步进工作区，返回工作区内的相对路径 */
export function syncFileToWorkspace(fileId: string, userId?: string) {
  return request<{ status: string; path: string }>("/files/sync-workspace", {
    method: "POST",
    body: JSON.stringify({ file_id: fileId, user_id: userId ?? "" }),
  });
}

/** 生成工作区文件的下载地址（浏览器直接访问以下载） */
export function workspaceDownloadUrl(path: string, userId?: string) {
  return `${API_BASE}/files/download?path=${encodeURIComponent(path)}&user_id=${encodeURIComponent(userId ?? "")}`;
}

/** 生成原始上传文件的下载地址 */
export function originalDownloadUrl(name: string, userId?: string) {
  return `${API_BASE}/files/download-original?name=${encodeURIComponent(name)}&user_id=${encodeURIComponent(userId ?? "")}`;
}

// ==================== 类型定义 ====================
// ==================== API 客户端 ====================
const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080";
async function request(path, init) {
    const response = await fetch(`${API_BASE}${path}`, {
        headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
        ...init,
    });
    if (!response.ok) {
        const text = await response.text();
        throw new Error(text || `Request failed with status ${response.status}`);
    }
    return (await response.json());
}
// ==================== 图谱相关 ====================
export function uploadNote(payload) {
    return request("/upload-note", {
        method: "POST",
        body: JSON.stringify(payload),
    });
}
/** 使用 LangChain 诊断流水线上传笔记（NER -> FactCheck -> Supplement） */
export function uploadNoteLangChain(payload) {
    return request("/upload-note-langchain", {
        method: "POST",
        body: JSON.stringify({ ...payload, use_langchain: true }),
    });
}
export function getGraphAll(params) {
    const query = new URLSearchParams();
    if (params?.user_id)
        query.set("user_id", params.user_id);
    if (params?.file_id)
        query.set("file_id", params.file_id);
    if (params?.file_group_id)
        query.set("file_group_id", params.file_group_id);
    const qs = query.toString();
    return request(`/graph/all${qs ? "?" + qs : ""}`);
}
export function getGraphPath(concept, userId, maxDepth = 3) {
    const params = new URLSearchParams({ concept, maxDepth: String(maxDepth) });
    if (userId)
        params.set("user_id", userId);
    return request(`/graph/path?${params.toString()}`);
}
export function getNodeNeighbors(nodeId, userId, depth = 1) {
    const params = new URLSearchParams({ node_id: nodeId, depth: String(depth) });
    if (userId)
        params.set("user_id", userId);
    return request(`/graph/neighbors?${params.toString()}`);
}
export function explainConcept(payload) {
    return request("/graph/explain", {
        method: "POST",
        body: JSON.stringify(payload),
    });
}
// ==================== 文件管理 ====================
export function listUserFiles(userId) {
    const query = userId ? `?user_id=${encodeURIComponent(userId)}` : "";
    return request(`/files${query}`);
}
export function createFile(name, fileGroupId, userId) {
    return request("/files/create", {
        method: "POST",
        body: JSON.stringify({ name, file_group_id: fileGroupId, user_id: userId }),
    });
}
export function createFileGroup(name, userId) {
    return request("/files/group/create", {
        method: "POST",
        body: JSON.stringify({ name, user_id: userId }),
    });
}
export function deleteFile(fileId, userId) {
    return request(`/files/delete?file_id=${encodeURIComponent(fileId)}&user_id=${userId ?? ""}`, {
        method: "DELETE",
    });
}
export function deleteFileGroup(groupId, userId) {
    return request(`/files/group/delete?group_id=${encodeURIComponent(groupId)}&user_id=${userId ?? ""}`, {
        method: "DELETE",
    });
}
export function renameFile(fileId, newName, userId) {
    return request("/files/rename", {
        method: "PUT",
        body: JSON.stringify({ file_id: fileId, new_name: newName, user_id: userId }),
    });
}
export function renameFileGroup(groupId, newName, userId) {
    return request("/files/group/rename", {
        method: "PUT",
        body: JSON.stringify({ group_id: groupId, new_name: newName, user_id: userId }),
    });
}
export function addFileToGroup(fileId, groupId, userId) {
    return request("/files/add-to-group", {
        method: "POST",
        body: JSON.stringify({ file_id: fileId, group_id: groupId, user_id: userId }),
    });
}
export function togglePinFile(fileId, userId) {
    return request(`/files/pin?file_id=${encodeURIComponent(fileId)}&user_id=${userId ?? ""}`, {
        method: "PUT",
    });
}
export function togglePinFileGroup(groupId, userId) {
    return request(`/files/group/pin?group_id=${encodeURIComponent(groupId)}&user_id=${userId ?? ""}`, {
        method: "PUT",
    });
}
export function getConversation(params) {
    const query = new URLSearchParams();
    if (params.file_id)
        query.set("file_id", params.file_id);
    if (params.file_group_id)
        query.set("file_group_id", params.file_group_id);
    if (params.conversation_id)
        query.set("conversation_id", params.conversation_id);
    if (params.user_id)
        query.set("user_id", params.user_id);
    return request(`/conversation?${query.toString()}`);
}
export function saveMessage(payload) {
    return request("/conversation/message", {
        method: "POST",
        body: JSON.stringify(payload),
    });
}
export function deleteConversation(conversationId, userId) {
    return request(`/conversation?conversation_id=${encodeURIComponent(conversationId)}&user_id=${userId ?? ""}`, {
        method: "DELETE",
    });
}
// ==================== AI 对话 ====================
export function chatWithContext(payload) {
    return request("/graph/chat", {
        method: "POST",
        body: JSON.stringify(payload),
    });
}
export function getLearningPath(payload) {
    return request("/graph/learning-path", {
        method: "POST",
        body: JSON.stringify(payload),
    });
}

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "http://localhost:8080";
async function request(path, init) {
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
    return (await response.json());
}
export function uploadNote(payload) {
    return request("/upload-note", {
        method: "POST",
        body: JSON.stringify(payload)
    });
}
export function getGraphAll(userId) {
    const query = userId ? `?user_id=${encodeURIComponent(userId)}` : "";
    return request(`/graph/all${query}`);
}
export function getGraphPath(concept, userId, maxDepth = 3) {
    const params = new URLSearchParams({
        concept,
        maxDepth: String(maxDepth)
    });
    if (userId)
        params.set("user_id", userId);
    return request(`/graph/path?${params.toString()}`);
}
export function explainConcept(payload) {
    return request("/graph/explain", {
        method: "POST",
        body: JSON.stringify(payload)
    });
}

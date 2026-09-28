"""
AI 工作区文件工具

让 AI 学习导师拥有一个「专属工作区」——可以像操作真实文件一样
列出、读取、写入、搜索、重命名工作区内的文件。

与 `agent_tools.py` 里基于内存文档的工具不同，这里的每个操作都会
真正落到用户的独立目录（Go 侧负责隔离与越权校验），因此：
- 修改结果可以被用户下载走
- 多个文件之间可以互相引用、批量处理

实现方式：这些工具通过 HTTP 回调 Go 网关的 /workspace/* 接口，
权限边界、路径清洗都由 Go 侧统一保证（单一事实来源），
Python 侧不直接触碰文件系统。
"""

from __future__ import annotations

import contextvars
import json
import logging
import os
import threading
from collections import OrderedDict
from typing import Optional

import httpx
from langchain_core.tools import tool

logger = logging.getLogger(__name__)

# Go 网关地址（与其它模块共用同一环境变量）
GATEWAY_BASE_URL = os.getenv("GO_GATEWAY_URL", "http://localhost:8080")
_WORKSPACE_TIMEOUT = float(os.getenv("WORKSPACE_TOOL_TIMEOUT_SECONDS", "30"))

# 当前会话对应的用户，由 agent 在每次对话前注入。
#
# 关键设计：用户身份**不能**由 LLM 通过工具参数传入（既不可靠也可被提示词篡改），
# 因此这里维护「当前活跃会话」的服务端状态，由 ReactChatAgent 在 invoke 前设置。
# 工具签名中保留 conversation_id 只是为了兼容显式传参，绝大多数情况无需 LLM 填写。
#
# ⚠️ 并发安全说明（原实现存在跨用户串号缺陷）：
# FastAPI 对 `def`（同步）端点使用线程池执行，多个用户可能**同时**在对话。
# 原实现用两个模块级变量保存「当前用户」，于是出现两个问题：
#   1) 并发的两个请求会互相覆盖 _active_conversation_id；
#   2) conversation_id 为空串时所有用户共用同一个 key ""，后者顶掉前者。
# 结果：A 的请求可能以 B 的身份读写工作区（跨租户越权）。
#
# 修复：
#   - 「当前会话」放入 contextvars，每个请求/线程各自独立，不再互相覆盖；
#   - conversation_id -> user_id 改为线程安全且**有容量上限**的 OrderedDict（LRU），
#     既保持显式传参可查，也避免长期运行后条目无界增长。
_active_conversation_id: contextvars.ContextVar[str] = contextvars.ContextVar(
    "active_conversation_id", default=""
)

_session_user: "OrderedDict[str, str]" = OrderedDict()
_session_lock = threading.Lock()
_MAX_SESSION_ENTRIES = 2000


def set_active_conversation(conversation_id: str) -> None:
    """标记当前正在处理的会话（服务端状态，LLM 无法干预）。"""
    _active_conversation_id.set(conversation_id or "")


def set_workspace_user(conversation_id: str, user_id: str) -> None:
    """绑定会话 -> 用户，后续工具调用据此确定工作区。"""
    if not conversation_id:
        # 空会话 ID 不能当作共享 key，否则所有用户会挤在同一个槽位互相覆盖。
        _active_conversation_id.set("")
        return
    _active_conversation_id.set(conversation_id)
    with _session_lock:
        _session_user[conversation_id] = user_id or ""
        _session_user.move_to_end(conversation_id)
        while len(_session_user) > _MAX_SESSION_ENTRIES:
            _session_user.popitem(last=False)


def get_workspace_user(conversation_id: str = "") -> str:
    cid = conversation_id or _active_conversation_id.get()
    if not cid:
        return ""
    with _session_lock:
        return _session_user.get(cid, "")


def _current_conversation_id(explicit: str = "") -> str:
    """确定本次工具调用应使用的会话：显式传入优先，否则用服务端记录的活跃会话。"""
    return explicit or _active_conversation_id.get()


def _call(method: str, path: str, user_id: str, **kwargs) -> dict:
    """调用 Go 工作区接口。失败时抛出带可读信息的异常。"""
    if not user_id:
        raise ValueError("当前会话未绑定用户，无法访问工作区")

    url = f"{GATEWAY_BASE_URL.rstrip('/')}{path}"
    params = dict(kwargs.pop("params", {}))
    params["user_id"] = user_id

    try:
        with httpx.Client(timeout=_WORKSPACE_TIMEOUT) as client:
            resp = client.request(method, url, params=params, **kwargs)
    except Exception as exc:  # noqa: BLE001
        raise RuntimeError(f"工作区服务不可用：{exc}") from exc

    body = resp.text
    if resp.status_code >= 400:
        detail = body
        try:
            parsed = json.loads(body)
            detail = parsed.get("error") or parsed.get("detail") or body
        except Exception:
            pass
        raise RuntimeError(str(detail))

    try:
        return json.loads(body) if body else {}
    except Exception:
        return {}


def _resolve_user(conversation_id: str) -> str:
    cid = _current_conversation_id(conversation_id)
    user_id = get_workspace_user(cid)
    if not user_id:
        # 未绑定用户时给出明确指引，避免 AI 反复重试
        raise RuntimeError("当前会话没有关联用户，无法使用工作区文件功能")
    return user_id


# ==================== 工作区工具 ====================

@tool
def workspace_list_files(conversation_id: str = "") -> str:
    """
    列出当前用户工作区中的所有文件。

    当你需要知道「有哪些文件可以操作」时使用。
    典型场景：用户说「把笔记里的错误都改一下」，你先列文件确认范围。

    Args:
        conversation_id: 会话ID（系统自动填入，无需手动传递）
    """
    user_id = _resolve_user(conversation_id)
    data = _call("GET", "/workspace/files", user_id)
    files = data.get("files", [])
    if not files:
        return "工作区当前没有文件。用户需要先上传笔记（上传后会自动同步到工作区）。"
    lines = [f"- {f['name']}（{f['size']} 字节，更新于 {f['mod_time']}）" for f in files]
    return "工作区文件列表：\n" + "\n".join(lines)


@tool
def workspace_read_file(path: str, conversation_id: str = "") -> str:
    """
    读取工作区中某个文件的完整内容。

    Args:
        path: 文件在工作区中的路径（由 workspace_list_files 获得）
        conversation_id: 会话ID（系统自动填入，无需手动传递）
    """
    user_id = _resolve_user(conversation_id)
    data = _call("GET", "/workspace/file", user_id, params={"path": path})
    content = data.get("content", "")
    if not content.strip():
        return f"文件 {path} 是空的。"
    return f"文件 {path} 的内容：\n\n{content}"


@tool
def workspace_write_file(path: str, content: str, conversation_id: str = "") -> str:
    """
    把内容写入工作区中的文件（覆盖原内容）。

    重要：这会直接修改用户的文件，且用户可以下载修改后的结果。
    因此写入前必须先 workspace_read_file 读取现有内容，基于全文改写，
    不要只凭记忆拼一份新内容，以免丢失原有信息。

    Args:
        path: 文件路径（须已存在，或是一个合理的 .md 文件名）
        content: 写入的完整新内容
        conversation_id: 会话ID（系统自动填入，无需手动传递）
    """
    user_id = _resolve_user(conversation_id)
    _call("PUT", "/workspace/file", user_id, json={"path": path, "content": content})
    return f"已写入文件 {path}（{len(content)} 字符）。用户可在界面上直接下载该文件。"


@tool
def workspace_search(keyword: str, conversation_id: str = "") -> str:
    """
    在工作区的所有文件中搜索关键字，返回所在文件与行号。

    用途：定位某个知识点出现在哪些笔记里，避免把整份笔记读进上下文。

    Args:
        keyword: 要搜索的关键字
        conversation_id: 会话ID（系统自动填入，无需手动传递）
    """
    user_id = _resolve_user(conversation_id)
    data = _call("GET", "/workspace/search", user_id, params={"keyword": keyword})
    hits = data.get("hits", [])
    if not hits:
        return f"工作区中没有找到「{keyword}」。"
    lines = [f"- {h['file']}:{h['line']}  {h['text']}" for h in hits]
    return f"找到 {len(hits)} 处匹配：\n" + "\n".join(lines)


@tool
def workspace_rename_file(path: str, new_name: str, conversation_id: str = "") -> str:
    """
    重命名工作区中的文件（仅改名，不移动目录）。

    Args:
        path: 原文件路径
        new_name: 新的文件名（只能是文件名，不能包含路径分隔符）
        conversation_id: 会话ID（系统自动填入，无需手动传递）
    """
    user_id = _resolve_user(conversation_id)
    data = _call("PUT", "/workspace/rename", user_id, json={"path": path, "new_name": new_name})
    return f"已重命名为 {data.get('path', new_name)}"


def get_workspace_tools() -> list:
    """返回工作区文件工具集（供 Agent 绑定）。"""
    return [
        workspace_list_files,
        workspace_read_file,
        workspace_write_file,
        workspace_search,
        workspace_rename_file,
    ]

# AGENTS.md

> 所有 AI Coding Agent（Codex / Claude Code / Cursor / OpenCode 等）修改本仓库时必须遵守的项目级规则。  
> 先读：`PROJECT_CONTEXT.md`、`ARCHITECTURE.md`、`PROJECT_STATUS.md`、`README.md`、`.cursor/rules/*`。

---

## 1. Before Coding

任何修改前必须：

1. 阅读 `PROJECT_CONTEXT.md` 与 `PROJECT_STATUS.md`，确认功能真实状态（DONE / PARTIAL / PLANNED）。  
2. 定位真实调用链：Frontend → Go → Python / Neo4j。  
3. 阅读将改动的相关层源码（不要只读 README 或申请书）。  
4. 搜索是否已有同类实现（诊断链、文件 API、路径 DFS、Chat Agent 等）。  
5. 再决定**最小**修改方案。  

**禁止**看到需求后立刻新建一套平行系统（第二套图谱管线、第二套 Agent 框架、第二套文件管理）。

---

## 2. Project Architecture Rules

保持职责边界：

```text
Frontend (Vue)  →  Go Gateway  →  Python AI Engine
                              ↘  Neo4j
```

| 层 | 应负责 | 不应负责 |
|----|--------|----------|
| Vue | UI、交互、G6、调用 Go API、本地展示态 | 直连 Neo4j、直连 DeepSeek、业务 MERGE 逻辑 |
| Go | API、校验、编排、Neo4j、代理 Python、路径 DFS | Prompt 工程、LangChain Agent 内部推理 |
| Python | 切分、Agent、Prompt、LLM、结构化诊断/对话 | 作为前端 BFF、长期图谱权威存储 |
| Neo4j | 图与元数据持久化 | 业务规则 UI、LLM 调用 |

前端默认只打 Go（`VITE_API_BASE_URL` / `localhost:8080`）。

---

## 3. Product Rules

来源：`.cursor/rules/00-product-design.md`

AGENT / Learning Graph 是 **AI Native Learning Platform**。

必须坚持：

- **Graph 是主要 Workspace**（视觉中心）  
- **AI 是 Learning Copilot**（导师，不是客服主角）  
- **文件只是入口**  

禁止把产品做成：

- ChatGPT Clone（聊天占主屏）  
- Admin Dashboard / 若依式后台  
- 纯文件管理器 / CMS  

任何新功能必须回答：是否帮助用户**建立知识体系**？若否，默认不做或降级。

---

## 4. UI Rules

修改前端前必须同时阅读：

- `.cursor/rules/00-product-design.md`  
- `.cursor/rules/01-ui-components.md`  

并遵守现有三栏与 Tailwind 设计系统。

禁止：

- 改成 Ant Design / Element Plus 后台风格  
- 随意增加传统左侧 Admin Sidebar 取代当前文件 Notebook 语义  
- 让 AI Chat 抢占 Knowledge Graph 主视觉  
- 滥用渐变、重阴影、无意义 Card 堆砌  
- 破坏 Graph > Report > Chat > File 的信息层级  
- 混用非 Lucide 图标体系  

---

## 5. Cross-Service Changes

若修改数据结构（节点、边、status、API 字段）：

必须检查并对齐：

1. `frontend/src/api/graph.ts`（及消费方）  
2. `backend-go/internal/model/graph.go`（及 service/repository）  
3. Python Pydantic（`app/schemas/*`、`langchain_agent/schemas/*`、router 模型）  
4. Neo4j 属性 / 关系 / Cypher  

**禁止**只改其中一个服务。

---

## 6. API Changes

修改 API 前检查完整链路：

Caller（Frontend）→ Router → Controller → Service → Repository / Python → 响应消费方。

除非用户明确要求破坏性变更：

- 保持现有路径与字段兼容  
- 新增优于重命名；弃用需标注  

已知主路径：`POST /upload-note-langchain`、`GET /graph/all`、`/graph/path`、`/graph/chat`、`/graph/learning-path`。

---

## 7. Neo4j Rules

改图模型前检查：

- 已有 Labels：`Concept`, `File`, `FileGroup`, `Conversation`, `Message`  
- 关系类型与 `sanitizeRelType`  
- `/graph/path` **写死**过滤 `PREREQUISITE_OF`  
- Repository Cypher 与前端边/节点 ID 语义（邻居边曾用内部 ID）  

避免：

- 后端写 `PREREQUISITE_OF`，前端或路径按另一名字理解  
- 只改 Agent 枚举不改入库聚合  
- 假设删除 File 会自动删除 Concept（当前**不会**）  

---

## 8. AI / LangChain Rules

修改 Agent 时，**不能**只改 Prompt 就宣称完成。必须检查：

Agent Input → Prompt → Tools（如有）→ Structured Output / Parser → API Response → Go Contract → Frontend Consumer  

现有 Agent 职责：

| Agent | 职责 |
|-------|------|
| NERAgent | 抽取实体与关系 |
| FactCheckAgent | 事实校验 |
| SupplementAgent | 逻辑补全 |
| ReactChatAgent | 对话与笔记工具 |

禁止创建与上述职责重叠的新 Agent。  
Learning Path 当前是 Prompt+LLM，不是第五个「诊断 Agent」；若升级，先论证必要性。

诊断链编排中心：`diagnosis_chain.py`。

---

## 9. Minimal Change Principle

默认：**最小改动**。

禁止：

- 因一个 Bug 重构整个 monorepo  
- 无关格式化、大规模重命名、无请求的架构升级  
- 「顺手」引入 Pinia/Vue Router/新 UI 库（除非任务明确要求）  

---

## 10. Existing Feature Protection

修改前确认功能在 `PROJECT_STATUS.md` 中的状态。

已工作的核心流程必须优先保持兼容：

- Upload（LangChain 诊断上传）  
- Graph 可视化  
- AI Chat  
- Learning Path（path + guidance）  
- Neo4j 写入与查询  

不允许为实现新功能破坏上述闭环。

---

## 11. Testing

以仓库真实脚本为准（不要编造命令）。

| 检查 | 命令 / 方式 |
|------|-------------|
| Frontend Build | `cd frontend && npm run build`（含 `vue-tsc -b`） |
| Frontend Dev | `npm run dev` |
| Go Build | `cd backend-go && go build .` |
| Go Test | 目前几乎无测试；若新增请 `go test ./...` |
| Python Syntax / Import | 在 venv 中启动或 `python -c "import main"`；无统一 pytest 套件 |
| AI Health | `GET :8000/health`、`GET :8000/api/langchain/health` |
| Go Health | `GET :8080/health` |
| Neo4j | Docker 容器 `agent-neo4j`；Bolt `7687` |

改 AI 相关后至少手动跑一条短 Markdown 诊断（可用 `ai-engine-python/debug_pipeline.py` 或 `test.md`）。

---

## 12. Secrets

禁止将以下内容写入代码或 Commit：

- API Key、Token、密码、私钥  

使用环境变量：

- Python：`DEEPSEEK_*`（dotenv）  
- Go：`NEO4J_*`、`PYTHON_SERVICE_URL` 等（进程环境；注意 Go **不**自动加载 `.env`）  

若发现示例文件含疑似真实密钥，应改为占位符，并轮换已暴露密钥（需人工处理）。

---

## 13. Completion Report

每次任务结束后输出：

### What Changed
### Files Changed
### Architecture Impact
### API Impact
### Database Impact
### Validation
### Remaining Issues
### Recommended Next Step

---

## 14. Documentation Sync

| 变化类型 | 更新文件 |
|----------|----------|
| 架构 / 调用链 / 组件边界 | `ARCHITECTURE.md` |
| 产品认知、模块职责、原则 | `PROJECT_CONTEXT.md` |
| 功能完成度、阶段、E2E 状态 | `PROJECT_STATUS.md` |
| 产品 / UI 长期规则 | `.cursor/rules/` |
| Agent 协作规则本身 | `AGENTS.md` |

不要更新业务代码却让文档仍描述旧架构。

---

## 15. Most Important Rule

在认为「当前架构不好，我要重新设计一套更好的」之前，必须先证明：

**现有架构无法满足需求。**

默认行为：

**理解现有系统 → 延续现有架构 → 最小必要修改。**

而不是：

**看到需求 → 重新造一套。**

本仓库已有：三栏 Workspace、Go DDD 网关、LangChain 诊断链、Neo4j MERGE、G6 三态图。优先扩展它们。

---

## Quick Pointers

| 任务 | 入口 |
|------|------|
| 产品是什么 | `PROJECT_CONTEXT.md` §1–2 |
| 怎么跑通 | `ARCHITECTURE.md` §14、`README.md` |
| 做到哪了 | `PROJECT_STATUS.md` |
| 改图谱 UI | `frontend/src/graph/g6-config.ts`, `view/main.vue` |
| 改 Agent | `ai-engine-python/app/langchain_agent/` |
| 改入库 | `backend-go/internal/repository/graph_repository.go` |

---

**Last Updated:** 2026-09-12  
**Commit baseline:** `b142bd3ae1c2d82fe033fca047549a5fc05f8b13`

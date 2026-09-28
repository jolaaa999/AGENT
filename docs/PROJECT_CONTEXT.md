# PROJECT_CONTEXT.md

> 专供未来 AI / Codex / Claude Code / Cursor Agent 阅读的项目总上下文。  
> **证据优先级**：当前实际代码 > 当前配置 > 最新 Git 修改 > `.cursor/rules` > README > 旧申请书 / 里程碑计划。  
> 无法确认处标记为 `TO VERIFY`。

---

## 1. Project Identity

| 字段 | 内容 |
|------|------|
| 项目名称（中文） | 基于 AI 智能体的专业图谱生成与个性化学习路径导航 |
| 项目英文名称（UI） | Learning Graph（见 `frontend/index.html` 标题） |
| 仓库 | https://github.com/jolaaa999/AGENT |
| 项目类型 | 大学生创新创业训练计划（创新训练项目） |
| 申报单位 | 武汉科技大学 · 计算机科学与技术学院 |
| 负责人 | 姜友康 |
| 指导教师 | 王晓峰 |
| 申报日期 | 2026-02-27 |
| 计划周期（申请书） | 2026-03 → 2028-03（24 个月） |
| 当前冲刺（里程碑表） | 2 个月 MVP：2026-07-15 → 2026-09-15 |

**一句话定义**  
把学生的 Markdown 学习笔记，经 AI 诊断后建成带状态（正确 / 错误 / 补全）的知识图谱，并在图谱工作区上完成逆向学习路径与 AI 伴学。

**核心用户**  
大学生（尤其复杂理工科）——有课堂笔记、需要理清前置依赖与知识漏洞的学习者。

**解决的问题**  
- 线性笔记导致知识碎片化，前置关系不可见  
- 笔记软件只存储、不诊断，错误认知会继续累积  
- 手工建图谱冷启动成本高  
- 高阶概念缺少「该先补什么」的个性化路径

---

## 2. Product Vision

系统**不是**：普通 AI Chat、文件管理器、CMS、后台管理系统、普通知识库。

系统**是**（见 `.cursor/rules/00-product-design.md`）：

| 定位 | 含义 |
|------|------|
| AI Learning Assistant | AI 知道课程 / 文件 / 图谱 / 节点 / 学习状态 |
| Knowledge Graph Platform | 图谱是主工作区，不是附属可视化 |
| Learning Copilot | AI 是学习导师，不是客服聊天窗 |

设计哲学：

- **Knowledge First**：知识结构优先于装饰与聊天  
- **Learning First**：一切服务于「建立知识体系」  
- **AI First**：AI 主动诊断、补全、导航，而非被动问答  
- **Knowledge Graph Workspace**：中间图谱永远是视觉中心  
- **Minimal / Information First**：安静、专注、低视觉噪音  

最终价值：把「死笔记」变成可诊断、可导航、可持续生长的个人认知网络。

---

## 3. Core User Journey

| 步骤 | 状态 | 代码证据 |
|------|------|----------|
| 用户粘贴 / 上传 Markdown | **Implemented** | `ImportPanel.vue`、`main.vue` `handleUpload` |
| 文档解析（AST 切分） | **Implemented** | `ai-engine-python/app/services/markdown_parser.py` |
| AI 知识抽取（实体 + 关系） | **Implemented** | `NERAgent` → `/api/langchain/diagnose` |
| 事实校验 / 错误标记 | **Implemented** | `FactCheckAgent`（失败可降级跳过） |
| 逻辑断层补全 | **Implemented** | `SupplementAgent`（失败可跳过） |
| Neo4j 图谱写入 | **Implemented** | Go `UpsertGraph` MERGE `:Concept` |
| 图谱可视化（G6 三态） | **Implemented** | `main.vue` + `g6-config.ts` |
| 文件 / 文件组管理 | **Implemented** | `/files*` + `FileSidebar.vue` |
| 节点交互 / 讲解 | **Partially Implemented** | 详情浮层 + `/graph/explain`；原文 snippets 计算了但 UI 未单独展示 |
| 知识漏洞分析展示 | **Partially Implemented** | 诊断摘要拼进 AI 聊天，无独立 Learning Report 页 |
| AI 辅导对话 | **Partially Implemented** | ReAct Chat + 工具；多轮历史存 Neo4j，但**未注入** LLM 上下文 |
| 个性化逆向学习路径 | **Partially Implemented** | `/graph/path` + `/graph/learning-path`；依赖 `PREREQUISITE_OF` 边质量 |
| Markdown 正文持久化与回看 | **Planned** | File 节点无 markdown 字段；每周计划进度 0 |
| GraphRAG / 跨学科隐性连线 | **Planned** | 申请书 / 周计划有；代码无 GraphRAG 管线 |
| Web Worker 大规模渲染 | **Planned** | 里程碑有；前端未实现 |
| 独立 / 融合模式用户开关 | **Partially Implemented** | 按 `file_id` / `file_group_id` 过滤 + 同名 MERGE；无显式 mode 开关 API |
| 持续学习 / 学习者画像 | **Planned** | 无掌握度模型 |

---

## 4. Core Features

| Feature | Purpose | Status | Frontend | Backend API | AI Engine | Storage |
|---------|---------|--------|----------|-------------|-----------|---------|
| Markdown 上传 / 粘贴 | 学习资料入口 | Implemented | ImportPanel / FileSidebar | `POST /upload-note-langchain` | diagnose | Neo4j Concept + File 元数据 |
| 旧版关系抽取 | 兼容路径 | Implemented（前端未接） | `uploadNote` 未调用 | `POST /upload-note` | `POST /api/parse` | Neo4j |
| 三 Agent 诊断 | 抽取 + 纠错 + 补全 | Implemented | 生成图谱按钮 | 经 Go 代理 | DiagnosisChain | — |
| Neo4j 图谱构建 | 长时记忆 | Implemented | — | Upsert | — | `:Concept` + 动态关系 |
| Knowledge Graph Workspace | 主工作区 | Implemented | 中栏 G6 | `GET /graph/all` | — | Neo4j |
| LightRAG 式核心节点展开 | 大图可读性 | Implemented（前端策略） | `extractCoreNodes` + neighbors | `GET /graph/neighbors` | — | Neo4j |
| 节点交互 / Tooltip | 状态与 reason | Partially | G6 tooltip + 浮层 | `/graph/explain` | explain | — |
| AI Chat | 图谱上下文伴学 | Partially | AiChatPanel | `POST /graph/chat` | ReactChatAgent | Conversation/Message；图上下文靠前端传入 + 工具搜索 |
| 诊断报告 | 上传后反馈 | Partially | 注入聊天 Markdown | 上传响应字段 | diagnose summary | 无独立表 |
| 逆向路径导航 | 前置技能树 | Partially | LearningNavPanel | `GET /graph/path` | — | 仅 `PREREQUISITE_OF` |
| 学习路径文案 | AI 指导 | Partially | 注入聊天 | `POST /graph/learning-path` | Prompt+LLM（非独立 Agent 类） | — |
| 会话持久化 | 按文件/组隔离 | Implemented | load/save/clear | `/conversation*` | — | Neo4j |
| 用户登录鉴权 | 多用户安全 | **未实现** | 本地记下 `user_id` | `X-User-ID` / 默认 `default_user` | — | 无 JWT |

---

## 5. Technology Stack

### Frontend
Vue 3、TypeScript、Vite 6、TailwindCSS 3、AntV G6 5、lucide-vue-next、marked  
**未使用**（与里程碑计划冲突）：Pinia、Vue Router、Axios、JWT

### Backend Gateway
Go（go.mod 标注 1.25.0）、Gin、gin-contrib/cors、neo4j-go-driver/v5  
分层：`controller` / `service` / `repository` / `model` / `router` / `config`  
**未使用**：Zap、JWT、本目录 Dockerfile

### AI Engine
Python、FastAPI、Uvicorn、LangChain / LangChain-OpenAI / LangGraph、OpenAI SDK、markdown-it-py、python-dotenv  
LLM：DeepSeek（OpenAI 兼容 API，`deepseek-chat`）

### Database
Neo4j 5.20（`docker-compose.yml`，容器名 `agent-neo4j`）

### Infrastructure
Docker Compose **仅编排 Neo4j**；前后端与 AI 引擎均为本地进程启动。

---

## 6. Repository Structure

```text
AGENT/
├── frontend/                 # Vue3 SPA：三栏 Knowledge Graph Workspace
│   └── src/
│       ├── view/main.vue     # 唯一页面枢纽（无 Router）
│       ├── components/       # FileSidebar / ImportPanel / LearningNavPanel / AiChatPanel
│       ├── api/graph.ts      # 全部 HTTP 客户端
│       └── graph/g6-config.ts
├── backend-go/               # Gin 网关 + Neo4j + 代理 Python
│   ├── main.go
│   └── internal/{controller,service,repository,model,router,config}
├── ai-engine-python/         # FastAPI + LangChain Agents
│   ├── main.py
│   └── app/{api,core,services,schemas,langchain_agent}
├── .cursor/rules/            # 产品与 UI 长期约束
│   ├── 00-product-design.md
│   └── 01-ui-components.md
├── docker-compose.yml        # Neo4j only
├── example-note.md           # 示例笔记
├── AGENT里程碑.xlsx          # MVP / 周计划（计划，非完成证明）
├── 附件2：…项目申请书….doc   # 大创申请书（愿景与 24 月计划）
├── README.md
├── LICENSE
├── image/                    # 示例相关静态资源
└── rust/                     # 目录存在；当前无有效业务代码（TO VERIFY 是否废弃）
```

---

## 7. System Components

### Frontend
单页三栏工作台：左文件、中图谱、右导入 / 导航 / AI。所有状态在 `main.vue` 的 `ref` 中。默认请求 `http://localhost:8080`。

### Go Backend
业务网关：校验请求、调用 Python、归一化 nodes/edges、Neo4j MERGE、文件与会话 CRUD、路径 DFS。默认端口 **8080**。

### Python AI Engine
Markdown 切分、旧版 parse/explain、LangChain 诊断流水线、ReAct 对话、学习路径文案生成。默认端口 **8000**。启动时强制校验 `DEEPSEEK_API_KEY`。

### Neo4j
持久化概念图、文件元数据、对话消息。Bolt **7687**，Browser **7474**。

### 通信关系

```text
Browser (Vue)
    │  HTTP JSON
    ▼
Go :8080  ──HTTP──►  Python :8000  ──HTTPS──►  DeepSeek API
    │
    └── Bolt ──► Neo4j :7687
```

前端**不直接**调用 Python 或 Neo4j。

---

## 8. AI Agent Design

**以代码为准，不要只信 README「三 Agent」。**

### 诊断流水线（3 Agents，顺序串行）

| Agent | 路径 | 职责 | 输入 | 输出 |
|-------|------|------|------|------|
| NERAgent | `app/langchain_agent/agents/ner_agent.py` | 实体 + 关系抽取 | chunks | `NEROutput` |
| FactCheckAgent | `.../fact_check_agent.py` | CoT 事实校验 | NER + markdown | `FactCheckOutput` |
| SupplementAgent | `.../supplement_agent.py` | 逻辑断层补全 | NER + markdown | `SupplementOutput` |

编排：`app/langchain_agent/chains/diagnosis_chain.py`  
入口：`POST /api/langchain/diagnose`  
Prompt：`app/langchain_agent/prompts/templates.py`  
Schema：`app/langchain_agent/schemas/agent_output.py`

说明：NER 注释写 ReAct，实现为 `Prompt | LLM` + Pydantic 解析，**不是** tool-calling ReAct。

### 对话 Agent（第 4 个）

| Agent | 路径 | 职责 |
|-------|------|------|
| ReactChatAgent | `agents/react_chat_agent.py` | LangGraph `create_react_agent` + 工具 |

Tools（`tools/agent_tools.py`）：`read_current_markdown`、`edit_markdown`、`append_markdown_section`、`search_knowledge_graph`  
图谱上下文：写入进程内 `_doc_store`，由工具检索；`get_chat_prompt` **已定义但未接入**当前 chat 路径。

### 学习路径（非独立 Agent 类）

`POST /api/langchain/learning-path`：Prompt + ChatOpenAI 直调，输出 `guidance` 文本。依赖树由 Go DFS 提供。

### DeepSeek 调用点

- LangChain：`ChatOpenAI(base_url=DEEPSEEK_BASE_URL, ...)`  
- 旧路径：`app/services/deepseek_service.py`（OpenAI SDK）

### 聚合注意

- `CORRECTS` 关系在 Schema 中存在，聚合时**故意不写入边**（代码 `pass`）  
- FactCheck / Supplement 失败可跳过；NER 失败则整链失败  

---

## 9. Data Model

### Neo4j Labels（实际）

| Label | 主要属性 | 说明 |
|-------|----------|------|
| `:Concept` | `user_id`, `name`, `type`, `status`, `reason`, `file_id`, `file_group_id`, `definition`, `source`, … | MERGE 键：`user_id + name` |
| `:File` | `user_id`, `file_id`, `name`, `file_group_id`, `pinned`, timestamps | **无正文 markdown** |
| `:FileGroup` | `user_id`, `group_id`, `name`, `file_ids`, `pinned` | |
| `:Conversation` | `conversation_id`, `user_id`, `file_id`/`file_group_id`, `title` | |
| `:Message` | `message_id`, `role`, `content`, `timestamp` | `Conversation-[:CONTAINS]->Message` |

### Relationships（实际）

- Concept→Concept：动态类型（消毒后），常见 `PREREQUISITE_OF` / `BELONGS_TO` / `RELATED_TO` / `SUPPLEMENTS`  
- `CORRECTS`：Schema 有，入库聚合通常不产生  
- `PART_OF_GROUP` / `:Course`：**里程碑计划有，代码未实现**

### 图谱建立 / 查询 / 删除

- **建立**：上传 → Python diagnose → Go `normalizeLangChainData` → `UpsertGraph`  
- **查询**：`/graph/all`、`/graph/path`（仅 `PREREQUISITE_OF`）、`/graph/neighbors`  
- **删除**：有 File / FileGroup / Conversation 删除；**无**独立删除 Concept API；删文件**不**级联删概念（注释与 Cypher 不一致）

### Schema 不一致（重要）

1. 计划 Label `Course` vs 实际无  
2. 邻居边 Source/Target 使用 Neo4j 内部 ID，路径相关边使用概念名  
3. 旧 `/api/parse` 输出 `relations[]`，LangChain 输出 `nodes[]+edges[]`，Go 两套归一化  

---

## 10. Product & UI Principles

来源：`.cursor/rules/00-product-design.md`、`01-ui-components.md`

**必须遵守**

- Graph 是视觉中心；AI 是辅助；文件只是入口  
- 三栏 Workspace：左 Notebook/文件 · 中 Graph · 右 AI / Report / Detail  
- 不做传统 Admin Dashboard / 左侧沉重业务后台  
- 设计方向：Apple / Linear / Notion / Arc Browser  
- Minimal、Information First；禁止高视觉噪音、游戏化、花哨渐变发光  
- 信息层级：Graph > Learning Report > AI Chat > File  
- Graph Panel 深色背景、最大面积；AI Chat 类 ChatGPT/NotebookLM  
- 图标统一 Lucide；颜色语义化（Indigo primary 等）

这些是**长期产品约束**，不是临时 UI 偏好。

---

## 11. Important Decisions

### DECISION-001 — 使用 Neo4j
**Decision**：图原生数据库承载概念与多跳依赖。  
**Reason**：申请书与实现一致——逆向技能树需要多跳遍历。  
**Impact**：路径查询、MERGE 去重、跨文件融合都以图模型为中心。

### DECISION-002 — Go + Python 双后端
**Decision**：Go 做网关与 Neo4j；Python 做 LLM/Agent。  
**Reason**：Go 适合 API/并发与图驱动；Python 适合 LangChain/NLP 生态。  
**Impact**：跨语言 DTO 必须同步；前端只对 Go。

### DECISION-003 — AI Engine 独立 FastAPI 服务
**Decision**：Python 单独进程 `:8000`。  
**Reason**：LLM 依赖与超时隔离；可独立迭代 Prompt/Agent。  
**Impact**：Go 通过 HTTP 代理；本地需同时启动三端 + Neo4j。

### DECISION-004 — Knowledge Graph 为 Workspace 中心
**Decision**：单页三栏，中栏 G6 最大。  
**Reason**：产品规则 Knowledge First。  
**Impact**：新功能不得把 Chat 或文件列表做成主角。

### DECISION-005 — 诊断状态三态编码
**Decision**：`correct` / `error` / `supplement` + `reason`。  
**Reason**：主动诊断是产品差异点。  
**Impact**：前后端、Agent Schema、G6 样式必须对齐该枚举。

### DECISION-006 — 用户身份暂用 user_id 字符串
**Decision**：无 JWT；`X-User-ID` / body / 默认 `default_user`。  
**Reason**：MVP 先跑通闭环。  
**Impact**：非安全多租户；勿当成已完成鉴权。

---

## 12. Current State

**截至代码审计时（commit `b142bd3`，分支 `main`；工作区另有未提交的 G6/`main.vue` 修改）**

### 已能工作（有完整调用链）
- Markdown → LangChain 诊断 → Neo4j → G6 三态渲染  
- 文件 / 文件组 CRUD、置顶、入组、按文件/组拉图  
- 节点邻居展开、路径查询、学习路径文案进聊天  
- AI Chat（含可选图片）、会话存 Neo4j  
- 概念讲解 API  

### Demo / 半完成
- 「登录」仅为本地 user_id  
- 诊断报告 = 前端拼 Markdown 进聊天  
- 节点讲解先本地模板再调 API  
- LightRAG 为核心节点抽样，非论文级 GraphRAG  

### 仅有 API / 前端未接
- `POST /upload-note`（非 LangChain）  
- `POST /files/create`（LangChain 上传可自动建文件）  

### 计划未接通
- Markdown 正文入库回看  
- 真正 GraphRAG  
- Web Worker / 千级节点优化  
- 多轮对话注入模型  
- AI 改笔记后自动再诊断刷新图谱（需用户手动再生成）  
- JWT / Zap / Pinia / Vue Router / Swagger / init.cypher  

### Mock
- 无前端假图谱数据集；LLM 为真实 DeepSeek（缺 key 则失败）  
- `debug_pipeline.py` 内置示例笔记仅用于调试  

---

## 13. Known Problems

### Confirmed Problems
- 删除文件不级联删除 Concept（注释承诺与实现不符）  
- Chat 多轮历史不进入 LLM；仅工具 + 当前消息  
- 学习路径强依赖 `PREREQUISITE_OF`；若 NER 大量产出 `RELATED_TO`，路径会空  
- `.env.example`（Python）含疑似真实 API Key 字面量（安全风险）  
- Go **不**自动加载 `.env` 文件，仅读进程环境变量  

### Technical Debt
- 前端大量 `*.js` / `*.vue.js` 与源码并存  
- 旧 parse 路径与 LangChain 路径双轨  
- `get_chat_prompt` 死代码；健康检查 agents 列表缺 ReactChat  
- Contract 在 Go model / Python Pydantic / TS types 三处重复  
- 无自动化测试（前端/Go/Python）  

### Product Gaps
- 无独立 Learning Report 页  
- 无学习者画像 / 掌握度  
- 无跨学科隐性关联（非同名 MERGE）  
- 无移动端打磨 / Onboarding（里程碑后期项）  

### To Verify
- 生产/答辩环境 DeepSeek 配额与延迟是否可演示稳定  
- G6 v5 部分 API（`focusElement` 等）在当前版本的完整行为  
- `rust/` 目录用途  
- 未提交的 `g6-config` / `main.vue` 改动是否为进行中的图谱优化  

---

## 14. AI Handoff Notes

### 接手前必读
1. 本文件 `PROJECT_CONTEXT.md`  
2. `ARCHITECTURE.md`、`PROJECT_STATUS.md`、`AGENTS.md`  
3. `.cursor/rules/00-product-design.md`、`01-ui-components.md`  
4. 真实调用链：`frontend/src/api/graph.ts` → `backend-go/internal/router/router.go` → `service` → Python `router.py` / Neo4j repository  

### 不能随意推翻
- 三栏 Graph-centric Workspace  
- `correct|error|supplement` 状态模型  
- Frontend → Go → (Python | Neo4j) 边界  
- DeepSeek 兼容 API 配置方式  

### 跨层修改检查清单
改 nodes/edges 字段 → 同时查：Agent Schema、Go model、TS types、G6 预处理、Cypher SET  
改关系名 → 同时查：NER Prompt、聚合、路径 Cypher（`PREREQUISITE_OF`）、前端边标签层级  

### 勿重复实现
- 诊断流水线已存在（不要再造第二套 NER）  
- 文件组 / 会话 API 已存在  
- 学习路径已有 Go DFS + Python guidance  

### 最容易误判
- README「三 Agent」→ 实际还有 ReactChat；learning-path 不是 Agent 类  
- 里程碑写 Pinia/JWT/GraphRAG/Web Worker → **多数未做**  
- UI 有「学习路径」→ 不是完整个性化画像系统  
- 申请书 24 月愿景 ≠ 当前 MVP 完成度  

---

## 15. Project Vocabulary

| Term | Meaning |
|------|---------|
| Knowledge Graph | Neo4j 中以 Concept 为节点的个人知识图，及前端 G6 视图 |
| Workspace | 三栏主界面；中栏图谱为核心 |
| Learning Path | 逆向前置依赖树 + AI `guidance` 文案 |
| Learning Report | 产品概念上的诊断/学习报告；当前多为聊天内 Markdown 摘要 |
| Agent | LangChain/LangGraph 智能体组件（NER / FactCheck / Supplement / ReactChat） |
| Diagnosis Chain | NER→FactCheck→Supplement 串行流水线 |
| Knowledge Node / Concept | 图谱概念节点（`:Concept`） |
| Graph Context | 提供给 AI 的节点/边信息（前端 JSON 或工具检索） |
| status | `correct` \| `error` \| `supplement` |
| File / FileGroup | 笔记文件元数据与分组；组内可共享 `file_group_id` 过滤 |
| LightRAG 模式 | 前端先展示核心节点再按需展开邻居（非完整 GraphRAG 产品） |
| MERGE 融合 | 同 `user_id+name` 概念合并；非跨学科隐性推理 |

---

## Document Meta

| 字段 | 值 |
|------|----|
| **Last Updated** | 2026-09-12 |
| **Branch** | `main` |
| **Commit** | `b142bd3ae1c2d82fe033fca047549a5fc05f8b13`（每周计划更新） |
| **Working Tree** | 有未提交修改：`frontend/src/graph/g6-config.*`、`frontend/src/view/main.vue` |
| **Current Project Stage** | 2 个月 MVP 冲刺末期（计划窗口至 2026-09-15）；核心「上传→诊断→入库→可视化→基础导航/对话」已通，申请书中后期能力（GraphRAG、画像、正文持久化、性能优化、正式鉴权）大多未完成 |

---

## 附录：计划 vs 代码冲突摘要

| 来源说法 | 代码事实 |
|----------|----------|
| Pinia + Vue Router + Axios + JWT | 无；单页 + fetch + user_id 字符串 |
| Zap + Dockerfile（Go） | 无 |
| `/api/diagnose`、`/api/graph/skill-tree` | 实际为 `/api/langchain/diagnose`、`/graph/path` |
| Course / PART_OF_GROUP / CORRECTS 入库 | Course/PART_OF_GROUP 无；CORRECTS 边不写 |
| GraphRAG | 未实现 |
| Web Worker 力导向 | 未实现 |
| Markdown 长时正文记忆 | File 无正文；仅概念与聊天消息持久化 |
| 三 Agent | 诊断 3 + 对话 1；另有 learning-path 直调 LLM |

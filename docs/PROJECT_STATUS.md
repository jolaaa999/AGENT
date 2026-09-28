# PROJECT_STATUS.md

> 回答：这个大创项目截至现在到底做到哪一步了？  
> **计划来源**（申请书 / 里程碑）≠ **完成证明**（代码与联调证据）。  
> Completion 为 `Estimated from repository state`，非精确项目管理数据。

---

## 1. Current Milestone

| 维度 | 判断 |
|------|------|
| 日历位置 | 2026-09-12，处于 **2 个月 MVP 窗口末期**（计划 2026-07-15 → 2026-09-15） |
| 申请书大阶段 | 仍属前期落地（申请书总周期至 2028-03）；MVP 冲刺不等于结题 |
| 代码阶段判断 | **里程碑 1～3 主体能力已落地**；里程碑 4（集成测试报告、文档体系、结题材料）进行中/不完整 |
| 一句话 | 核心闭环「上传 → AI 诊断 → Neo4j → G6 → 基础路径/对话」可演示；高级项（GraphRAG、画像、正文持久化、Web Worker、正式鉴权）大多未做 |

里程碑 Excel 中「实际开始/结束」列为空，故阶段判断**不以 Excel 勾选为准**，而以仓库代码为准。

---

## 2. Overall Progress

| Area | Status | Completion | Notes |
|------|--------|----------:|-------|
| Frontend | PARTIAL | ~75% | 三栏 Workspace + G6 + 文件 + Chat；无 Router/Pinia；无 Web Worker |
| Go Backend | PARTIAL | ~80% | 图谱/文件/会话/代理 API 齐全；无 JWT/Zap/Dockerfile；删文件不级联 |
| AI Engine | PARTIAL | ~75% | 诊断三 Agent + ReAct Chat + learning-path；无 GraphRAG；无测试 |
| Knowledge Graph | PARTIAL | ~70% | MERGE + 三态 + 按文件/组；无 Course；CORRECTS 边不写；正文不入库 |
| AI Agent | PARTIAL | ~70% | 诊断可用；Chat 工具可用；多轮记忆未进模型 |
| Learning Path | PARTIAL | ~55% | DFS + guidance 文案有；依赖 PREREQUISITE_OF；无画像 |
| Deployment | PARTIAL | ~30% | 仅 Neo4j compose；无全栈编排/云部署 |
| Testing | PARTIAL | ~10% | 几乎无自动化测试；有示例 md 与 debug 脚本 |
| Documentation | PARTIAL | ~40% | README 简略；本轮补充 CONTEXT/ARCH/STATUS/AGENTS |

*Completion: Estimated from repository state*

---

## 3. Implemented Features

仅列有实现证据者。

| Feature | Frontend | Backend | AI Engine | Database | Status | Evidence |
|---------|----------|---------|-----------|----------|--------|----------|
| Markdown 粘贴/上传生成图谱 | Y | `/upload-note-langchain` | diagnose | Concept MERGE | DONE | `main.vue` → Go → Python |
| AST Semantic Chunking | — | — | markdown_parser | — | DONE | `markdown_parser.py` |
| NER + 关系抽取 | — | 归一化 | NERAgent | Concept/Rel | DONE | `ner_agent.py` |
| 事实校验标错 | G6 红态 | status 入库 | FactCheckAgent | status/reason | DONE | 可降级跳过 |
| 知识补全节点 | G6 补全态 | 入库 | SupplementAgent | supplement | DONE | 可跳过 |
| 全图/按文件/按组查询 | Y | `/graph/all` | — | Cypher | DONE | repository |
| G6 三态渲染与布局 | Y | — | — | — | DONE | `g6-config.ts` |
| 文件/组 CRUD、置顶、入组 | Y | `/files*` | — | File/FileGroup | DONE | 入组会级联回填 Concept.file_group_id |
| 多格式上传解析 | Y | `/files/extract` | `/api/extract/to-markdown` | — | DONE | md/txt/docx/pdf/图片（识图） |
| 文件正文存取 | Y | `/files/markdown`、PUT `/files/content` | — | File.content | DONE | 支持组内聚合与编辑落盘 |
| 数学公式渲染 | Y | — | — | — | DONE | 前端 KaTeX，支持 $..$ / $$..$$ |
| 邻居展开 | Y | `/graph/neighbors` | — | Cypher | DONE | LightRAG 模式 |
| 逆向路径查询 | Y | `/graph/path` | — | PREREQUISITE_OF + 弱关联 | DONE | 分层：强依赖优先、关联边兜底；上限 12 并回传截断信息 |
| 学习路径文案 | 注入 Chat | `/graph/learning-path` | Prompt+LLM | — | DONE | 非独立 Agent 类 |
| AI 上下文对话 | Y | `/graph/chat` | ReactChatAgent | 消息可选存 | DONE | 工具可搜图/改 md，改动可落盘 |
| AI 工作区（文件沙箱） | Y | `/workspace/*` | 工作区工具 5 个 | 磁盘目录 | DONE | 按 user_id 隔离；越权路径一律拒绝 |
| 上传正文持久化 | Y | `/upload-note` | — | Neo4j | DONE | 追加到已有文件时也保存正文（此前会丢） |
| 原始文件归档 | Y | `/files/extract` | — | 工作区 original/ | DONE | 原件按 user_id 归档，可在工作区下载 |
| 可折叠侧边栏 | Y | — | — | localStorage | DONE | 折叠为图标窄栏；宽度<160px 自动折叠 |
| 画布白屏修复 | Y | — | — | — | DONE | graphRoot 改用内层容器；resize 后重绘；force 迭代 1800→300 |
| 邻居边端点修复 | Y | `/graph/neighbors` | — | — | DONE | 边 source/target 改用概念名，修复「展开失败：Node not found」 |
| 节点状态被边覆盖修复 | Y | `UpsertGraph` | — | — | DONE | 写入关系时不再 SET t.status，修复 error 节点全变 supplement |
| 测试样张 | Y | — | — | — | DONE | `测试样张-图谱.md`（上传用）+ `测试样张-使用说明.md`（步骤） |
| 全局Bug审计-删除级联 | Y | `DeleteFile` | — | — | DONE | 删文件同时删其概念节点+清理组内悬空id |
| 全局Bug审计-并发串号 | Y | workspace/agent tools | — | — | DONE | 全局变量改 contextvars，消除跨用户越权 |
| 全局Bug审计-会话内存 | Y | `_doc_store` | — | — | DONE | 改 LRU 上限 500，避免无界增长 |
| 全局Bug审计-静默成功 | Y | 删除/改名/更新接口 | — | — | DONE | 命中0行报错并映射 404 |
| 全局Bug审计-加密PDF | Y | `_convert_pdf` | — | — | DONE | 检查 decrypt 返回值，给出中文提示 |
| 全局Bug审计-悬空组引用 | Y | `AddFileToGroup` | — | — | DONE | 先校验文件与组均存在再写入 |
| 错误检测降级修复 | Y | fact_check_agent | — | — | DONE | status 只允许 correct/error，supplement 归一为 error（三层防护） |
| 节点重叠修复 | Y | g6-config.ts | — | — | DONE | collide 半径按节点尺寸自适应，不再写死 46 |
| 适应屏幕修复 | Y | main.vue | — | — | DONE | 设置 viewport padding，afterlayout 后自动适配 |
| 逆向导航计数恒为0 | Y | graph_repository.go | — | — | DONE | dependency_tree 未填 strength，前端 filter 恒 0；补齐 strong/via |
| 深度上限静默截断 | Y | graph_repository.go | — | — | DONE | 前端写 12 后端夹 6 且不回传 meta；统一为 6 并回传 meta |
| Meta 从未回传 | Y | graph_repository.go | — | — | DONE | PathResponse.Meta 一直为 nil，补齐 requested/applied/clamped |
| :8000 端口被占用 | Y | 环境 | — | — | DONE | 外部 python3 app.py 抢占 127.0.0.1:8000 导致 learning-path 502 |
| 文件下载（改后稿 + 原件） | Y | `/files/download`、`/files/download-original` | — | 磁盘目录 | DONE | 原件在上传转 Markdown 时留存 |
| 会话按文件隔离 | Y | `/conversation*` | — | Conversation/Message | DONE | 组对话聚合组内全部正文 |
| 概念讲解 | 节点详情 | `/graph/explain` | `/api/explain` | — | DONE | 有本地兜底文案 |
| Neo4j Docker | — | — | — | compose | DONE | `docker-compose.yml` |
| 旧版 `/api/parse` | API 未接 | `/upload-note` | parse | 可写 | DONE（备用） | 前端主路径不用 |

---

## 4. Partially Implemented

| Item | 已有 | 断点 |
|------|------|------|
| Learning Report | 上传后诊断 Markdown 进聊天 | 无独立报告页/组件；无历史报告存储 |
| 节点原文定位 | `extractMarkdownSnippets` 有计算 | UI 未单独展示 snippets |
| 专注模式 / 路径可视化 | path + dim/focus + dagre | 里程碑中的粒子流动画等未做 |
| 独立/融合图谱 | file_id 过滤 + 同名 MERGE + 组 id | 无用户显式 mode 开关；无 skill-tree mode 参数 |
| AI 改笔记闭环 | Chat 可返回 `edited_markdown` 写回编辑器 | **不会**自动再 diagnose / 刷新图谱 |
| 多轮伴学 | 消息可存 Neo4j | LLM 每次只看当前轮；历史不注入 |
| 「登录」 | UI 可填 user_id；登录态写入 `localStorage`，刷新后自动恢复；提供「退出」 | 无鉴权、无 Token |
| 产品规则中的 Streaming/Skeleton | 有全屏 loading | 未达规则中的逐步流式 AI loading 完整度 |

---

## 5. Planned But Not Implemented

来源：申请书、`AGENT里程碑.xlsx` 每周计划、产品规则。**不保证未来一定做。**

- Markdown 正文持久化与回看/下载（周计划进度 0）  
- GraphRAG 检索增强流水线  
- 跨学科隐性关联（非仅同名 MERGE）  
- Web Worker 力导向 + 视口增量渲染  
- 学习者画像驱动路径  
- JWT / 用户管理体系  
- Pinia + Vue Router（里程碑脚手架项）  
- Zap 日志、Go Dockerfile、Swagger、init.cypher  
- Course 节点、PART_OF_GROUP、CORRECTS 边落地  
- 移动端优化、Onboarding  
- 结题报告 / 答辩 PPT / 用户手册（文档里程碑）  
- 软件著作权与论文（申请书后期）  

---

## 6. Current End-to-End Flows

### FLOW-01 — 上传诊断建图
Upload Markdown → AST → Diagnose Agents → Neo4j → G6  
**Status: WORKING**（需 DeepSeek Key + 四服务就绪）  
中断点：Key 缺失、超时、NER 全失败。

### FLOW-02 — 按文件/组浏览图谱
选文件/组 → `/graph/all` → 渲染  
**Status: WORKING**

### FLOW-03 — 逆向学习路径 + AI 指导
输入概念 → `/graph/path` → 高亮 → `/learning-path` → 聊天展示  
**Status: PARTIAL**  
中断点：缺少 `PREREQUISITE_OF` 边时树为空，guidance 不触发或质量差。

### FLOW-04 — AI 伴学对话
聊天 → Go → ReactChat → 工具搜图/改笔记 → 可选存消息  
**Status: PARTIAL**  
中断点：无多轮上下文；改笔记后需手动再生成图谱。

### FLOW-05 — 节点点击讲解
点击节点 → 本地兜底 → `/graph/explain`  
**Status: WORKING**（无 markdown 时仅兜底）

### FLOW-06 — 旧版 parse 上传
`/upload-note` → `/api/parse`  
**Status: UNKNOWN/UNUSED**（后端有，前端主路径未接）

### FLOW-07 — Markdown 回看原文
选文件加载历史正文  
**Status: BROKEN/缺失**（正文未入库；刷新后编辑器依赖当前会话内存）

---

## 7. Current APIs

### Working（前后端均有调用证据）
`/upload-note-langchain`, `/graph/all`, `/graph/path`, `/graph/neighbors`, `/graph/explain`, `/graph/chat`, `/graph/learning-path`, `/files`（及组/删/改/置顶/入组）, `/conversation*`

### Partial
- `/graph/path` + learning-path：功能通，数据依赖关系类型  
- `/graph/chat`：通，但记忆/自动再诊断不完整  

### Unused（后端有，前端未调）
`POST /upload-note`, `POST /files/create`

### Legacy
Python `POST /api/parse`（旧抽取）；仍被 Go `/upload-note` 使用  

### Unknown
生产环境稳定性、真实答辩数据集上的准确率 — **TO VERIFY**

---

## 8. Current AI Capabilities

| Capability | Reality |
|------------|---------|
| 文档解析 | DONE（标题/代码块 AST 切分） |
| 知识抽取 | DONE（NER） |
| 关系抽取 | DONE（枚举关系；非法回退 RELATED_TO） |
| 图谱诊断（纠错/补全） | DONE（可降级） |
| Chat | DONE（ReAct + 工具） |
| Graph Context | PARTIAL（工具检索 + 前端传 JSON；非全图进 prompt） |
| Learning Path 文案 | DONE（弱依赖树） |
| Multi-Agent | DONE（诊断串行 3 Agent + 独立 Chat Agent） |
| GraphRAG | PLANNED / 未实现 |
| 多轮历史注入 | PLANNED / 未实现 |
| 幻觉死锁检测完整链 | PARTIAL（有重试/校验；周计划「死锁检测」未单独实现） |

---

## 9. Current Frontend Pages

| Page | Purpose | Status | Connected API | Main Components |
|------|---------|--------|---------------|-----------------|
| 唯一 SPA（`main.vue`） | Knowledge Graph Workspace | DONE/PARTIAL | 见 §7 Working | FileSidebar, G6, ImportPanel, LearningNavPanel, AiChatPanel |

无多路由页面；无独立设置页 / 报告页 / 管理后台。

---

## 10. Current Problems

### P0 — 阻塞完整产品闭环
1. Markdown **正文不持久化** → 刷新后无法从文件节点回看原文，讲解/再诊断依赖内存中的 markdown。  
2. 学习路径对 `PREREQUISITE_OF` **过约束** → 演示可能「有图无路径」。  
3. 依赖外部 DeepSeek → Key/配额/网络失败则核心闭环中断。  

### P1 — 重要
1. 删文件不删概念 → 图谱脏数据。  
2. Chat 无多轮注入 / 改笔记不自动刷新图。  
3. `.env.example` 疑似含真实 Key；Go 不读 `.env` 易配置踩坑。  
4. 三端 Schema 手同步，易出合同 Bug。  

### P2 — 优化
1. 无自动化测试。  
2. 前端编译旁路 `*.js` 噪音。  
3. 死代码（`get_chat_prompt` 等）。  
4. 无 Web Worker / 大图性能方案。  

---

## 11. Technical Debt

| 类型 | 发现 |
|------|------|
| Mock | 无系统假数据；节点讲解有本地模板兜底 |
| Hardcoded | 置信度 0.6、核心节点 5、截断 8000 字、默认 user、RELATED_TO 回退 |
| Duplicate | 旧 parse vs diagnose；TS/Go/Python DTO 三份 |
| Missing Test | 无 `*_test.go` / pytest / 前端单测 |
| Missing Error Handling | Chat `saveMessage` 静默失败；部分 Agent 内部吞异常 |
| Temporary API | user_id 伪登录；CORS `*` |
| Schema inconsistency | 计划 vs 实际 Label/关系；邻居边 ID 语义 |
| Comment drift | DeleteFile 注释承诺级联删除但未实现 |

---

## 12. Recent Development

根据最近 Git 提交主题（不逐条抄写）：

- 图谱交互与布局优化（可拖动、分组、边穿过节点等问题迭代）  
- NER 长文本兜底与调试脚本 / 短测试材料  
- Prompt 调整  
- 前端状态驱动渲染、AI 前端 rules  
- 更早：前后端闭环、LangChain 初始化、DeepSeek 真调用、节点讲解、里程碑文档  

当前工作区未提交：`g6-config.js/ts`、`main.vue` —— 疑似图谱优化进行中。

---

## 13. Next Recommended Milestone

最多 5 项，服务「可稳定演示的 MVP 闭环」：

1. **Markdown 正文持久化**：File 存 content；选中文件回填编辑器（打通 FLOW-07）。  
2. **路径可用性加固**：关系抽取/后处理提升 `PREREQUISITE_OF` 比例，或路径查询兼容更多依赖语义。  
3. **上传后再诊断体验**：AI 编辑笔记后一键/自动刷新图谱；诊断报告面板化（不必再只塞聊天）。  
4. **数据卫生**：删文件级联或提供清理 Concept；演示用一键 reset 文档化。  
5. **演示包**：3～5 篇稳定笔记 + 录屏脚本 + 环境检查清单（Key/四服务健康检查）。  

---

## 14. Definition of MVP

最小可完整演示版本必须具备：

1. **Upload** — 用户可贴/传 Markdown  
2. **AI** — 诊断产出 correct/error/supplement  
3. **Knowledge Graph** — 写入 Neo4j 并在中栏可视化  
4. **Analysis** — 用户能看到错误/补全原因（tooltip 或报告）  
5. **Learning** — 至少一个高阶概念能展开前置路径或获得 AI 学习指导；AI 能基于当前图/笔记问答  

**当前相对 MVP**：1～4 基本具备；5 部分具备（路径不稳定、伴学无长记忆）。补齐 §13 的 1～2、5 后更接近「答辩可讲清闭环」的 MVP。

---

## 15. Demo Readiness

| 场景 |  readiness | 缺什么 |
|------|------------|--------|
| 老师检查 / 周进度 | **可用** | 讲清已实现 vs 计划；准备 1 篇稳定笔记现场跑 |
| 中期答辩 | **基本可演** | 需预录备用视频防 API 波动；强调架构与三 Agent；诚实说明未做 GraphRAG/画像 |
| 项目演示（展台） | **谨慎** | 需离线/缓存方案或稳定 Key；清理脏图数据流程；避免依赖冷门关系类型的路径 Demo |
| 最终答辩 | **不足** | 缺长周期验证、完整文档、性能与跨域融合、申请书后半程成果 |

---

## Document Meta

| 字段 | 值 |
|------|----|
| **Last Updated** | 2026-09-12 |
| **Current Commit** | `b142bd3ae1c2d82fe033fca047549a5fc05f8b13` |
| **Branch** | `main` |
| **Current Milestone** | MVP 冲刺末期：核心诊断建图可视化已通，集成交付与高级能力未完 |
| **Next Milestone** | 正文持久化 + 路径可靠性 + 演示包装（见 §13） |

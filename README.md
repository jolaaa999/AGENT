# 基于AI智能体的专业图谱生成与个性化学习路径导航

本仓库采用 Monorepo 结构，包含前端可视化系统、Go 网关服务、Python AI 解析引擎，以及本地开发所需的 Neo4j 数据库编排配置。

## 项目结构

```text
.
├── frontend/             # Vue3 + TypeScript + Vite + TailwindCSS + AntV G6
├── backend-go/           # Go + Gin + Neo4j Driver (DDD 分层)
├── ai-engine-python/     # FastAPI + LangChain Agent + DeepSeek 解析引擎
├── docker-compose.yml    # 一键启动 Neo4j
└── README.md
```


## 快速开始

1. 启动 Neo4j：

   ```bash
   docker compose up -d
   ```

2. 启动前端：

   ```bash
   cd frontend
   npm install
   npm run dev
   ```

3. 启动 Go 网关：

   ```bash
   cd backend-go
   go mod tidy
   go run .
   ```

4. 启动 AI 引擎（Python + LangChain Agent）：

   建议使用虚拟环境，避免依赖污染系统 Python：

   ```bash
   cd ai-engine-python
   python -m venv .venv

   # 激活虚拟环境
   # Linux / macOS:
   source .venv/bin/activate
   # Windows PowerShell:
   # .\.venv\Scripts\Activate.ps1

   pip install -r requirements.txt
   uvicorn main:app --reload --port 8000
   ```

   之后每次新开终端，先进入 `ai-engine-python` 并重新激活虚拟环境，再启动服务。

   启动后可访问：
   - `GET  /health` — 服务健康检查
   - `GET  /api/langchain/health` — LangChain 模块状态
   - `POST /api/parse` — 原始 DeepSeek 解析（OpenAI SDK）
   - `POST /api/langchain/diagnose` — LangChain 三 Agent 诊断流水线
   - `POST /api/langchain/chat` — AI 对话（基于图谱上下文）
   - `POST /api/langchain/learning-path` — 个性化学习路径指导

## 技术栈

- 前端：Vue3、TypeScript、Vite、TailwindCSS、AntV G6
- 后端网关：Go、Gin、Neo4j Go Driver
- AI 引擎：Python、FastAPI、Uvicorn、LangChain (Agent框架)、LangChain-OpenAI、OpenAI SDK、Markdown-It-Py
- 数据库：Neo4j（Docker 部署）

## 清空本地数据库
# 进入 Neo4j 容器执行 cypher-shell
docker exec -it agent-neo4j cypher-shell -u neo4j -p password "MATCH (n) DETACH DELETE n"
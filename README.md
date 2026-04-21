# 基于AI智能体的专业图谱生成与个性化学习路径导航

本仓库采用 Monorepo 结构，包含前端可视化系统、Go 网关服务、Python AI 解析引擎，以及本地开发所需的 Neo4j 数据库编排配置。

## 项目结构

```text
.
├── frontend/             # Vue3 + TypeScript + Vite + TailwindCSS + AntV G6
├── backend-go/           # Go + Gin + Neo4j Driver
├── ai-engine-python/     # FastAPI + DeepSeek(OpenAI SDK) 解析引擎
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

4. 启动 AI 引擎（Python）：

   ```bash
   cd ai-engine-python
   pip install -r requirements.txt
   uvicorn main:app --reload --port 8000


   cd d:/AGENT/ai-engine-python && .\venv\Scripts\Activate.ps1 && uvicorn main:app --reload --port 8000
   ```

## 技术栈

- 前端：Vue3、TypeScript、Vite、TailwindCSS、AntV G6
- 后端网关：Go、Gin、Neo4j Go Driver
- AI 引擎：Python、FastAPI、Uvicorn、OpenAI SDK、Markdown-It-Py

## 清空本地数据库
# 进入 Neo4j 容器执行 cypher-shell
docker exec -it agent-neo4j cypher-shell -u neo4j -p password "MATCH (n) DETACH DELETE n"
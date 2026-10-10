# Caddy Panel

可视化管理 Caddy 的开源面板:站点/反向代理表单化配置、HTTPS 证书状态、Caddyfile 校验与热加载、配置版本回滚、日志查看。

- **架构**:Go 后端(唯一事实源 SQLite + 渲染引擎)→ Caddyfile 渲染 → adapt 校验 → Admin API(`/load`)热加载,Caddyfile 落盘保障重启恢复。
- **技术栈**:Go 1.27 + Gin + GORM/SQLite;React 19 + TypeScript + Vite + Ant Design 6(依赖管理:Bun),中/英双语、亮/暗主题。
- **状态**:🚧 设计完成,待启动编码(M1 脚手架)。
- **License**:Apache-2.0

## 文档

| 文档 | 内容 |
|---|---|
| [AGENTS.md](AGENTS.md) | AI 代理/协作者入口:antd 6 查阅姿势、MCP 配置、swaggo 约定、常用命令、提交检查 |
| [实现方案调研](docs/research-implementation-plan.md) | Caddy 管理能力(Admin API/持久化/Go 库)、竞品对比、架构方案对比与推荐、风险对策 |
| [需求规格 PRD](docs/requirements.md) | 功能需求与验收标准(编号化)、非功能需求、M2/M3 占位、追溯矩阵 |
| [进度管控](docs/progress.md) | 当前状态、里程碑、任务看板、决策与变更日志 |
| [技术栈选型明细](docs/tech-stack.md) | 各层选型与版本、选型理由、前后端依赖清单、运行形态、工程规范 |
| [项目架构与目录设计](docs/project-structure.md) | 运行时架构、完整目录结构(规划)、关键数据流转、环境变量约定 |
| [M1 详细设计](docs/m1-design.md) | MVP 目标边界、数据模型、API 设计、配置管道、前端页面、任务拆解 |

## 快速开始(编码启动后可用)

```bash
# 后端
cd backend && go run ./cmd/panel        # :8080

# 前端(Bun)
cd frontend && bun install && bun run dev   # :5173,proxy /api → :8080

# 或 Docker
docker compose -f deploy/docker-compose.yml up -d
```

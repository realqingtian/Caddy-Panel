# Caddy Panel

可视化管理 Caddy 的开源面板:站点/反向代理表单化配置、HTTPS 证书状态、Caddyfile 校验与热加载、配置版本回滚、日志查看。

- **架构**:Go 后端(唯一事实源 SQLite + 渲染引擎)→ Caddyfile 渲染 → adapt 校验 → Admin API(`/load`)热加载,Caddyfile 落盘保障重启恢复。
- **技术栈**:Go 1.27 + Gin + GORM/SQLite;React 19 + TypeScript + Vite + Ant Design 6(依赖管理:Bun),中/英双语、亮/暗主题。
- **状态**:✅ M1(MVP)已完成 —— 登录鉴权、站点管理(反代/静态/重定向)、配置中心(版本/回滚/直编)、日志查看、漂移检测、Docker Compose 一键部署。
- **License**:Apache-2.0

## 功能(M1)

| 模块 | 能力 |
| --- | --- |
| 认证 | 单管理员登录(JWT)、连续失败锁定(阈值/时长运行时可调)、首启自动创建管理员 |
| 站点管理 | 反向代理(多上游/负载均衡/WebSocket/自定义响应头)、静态站点、重定向;保存即校验+热加载,失败不生效并透传 Caddy 错误原文;启停/删除;证书签发状态查看 |
| 配置中心 | 渲染产物预览与直接编辑(强制校验)、版本历史(成败都留痕)、任意成功版本一键回滚、Caddy 当前生效 JSON 查看 |
| 日志 | 访问/错误日志在线 tail(轮询可暂停,默认 200 行,上限 1000) |
| 仪表盘 | Caddy 存活/版本、站点计数、最近下发、配置漂移检测(DB/磁盘/运行态三方对比) |
| 系统设置 | Admin API 地址、Caddyfile 路径、日志路径、登录锁定参数、metrics 开关、全局自定义片段 |
| 界面 | 中/英双语即时切换(后端错误消息同步本地化)、亮/暗/跟随系统主题 |

## 快速开始

### Docker Compose(推荐)

```bash
git clone https://github.com/<you>/caddy-panel.git
cd caddy-panel/deploy

# 可选:预置管理员密码(未设置时随机生成,打印在 panel 启动日志中,仅一次)
export PANEL_ADMIN_PASSWORD=your-password

docker compose up -d
# 面板:  http://localhost:8080  (账号 admin)
# Caddy: 80/443
```

两容器组成:`caddy`(官方镜像,承载 80/443 流量)+ `panel`(单二进制,内嵌前端,管理 SQLite/Caddyfile/证书存储只读视图)。日志目录与证书存储以卷共享给 panel。

### 源码开发

依赖:Go 1.27+、Bun 1.2+、(可选)本机 Caddy 用于真实下发。

```bash
# 后端(:8080)
cd backend && go run ./cmd/panel

# 前端(:5173,/api 代理到 :8080)
cd frontend && bun install && bun run dev
```

开发态入口:面板 `http://localhost:5173`,Swagger `http://localhost:8080/swagger/index.html`。

## 配置(env)

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `PANEL_LISTEN` | `:8080` | 面板监听地址 |
| `PANEL_DB_PATH` | `./data/panel.db` | SQLite 路径 |
| `PANEL_ADMIN_URL` | `http://localhost:2019` | Caddy Admin API(`http(s)://` 或 `unix://`) |
| `PANEL_CADDY_ADMIN_BIND` | `localhost:2019` | 渲染全局块的 admin 绑定,须与面板可达地址一致 |
| `PANEL_CADDYFILE_PATH` | `./data/Caddyfile` | 渲染产物落盘路径 |
| `PANEL_JWT_SECRET` / `PANEL_JWT_TTL` | 随机(持久化)/ `24h` | JWT 密钥与有效期(仅 env,重启生效) |
| `PANEL_ADMIN_PASSWORD` | 随机 | 首次启动管理员密码(仅一次) |
| `PANEL_LOGIN_MAX_FAILS` / `PANEL_LOGIN_LOCK_WINDOW` | `5` / `1m` | 登录锁定(设置页可改,立即生效) |
| `PANEL_LOG_ACCESS_PATH` / `PANEL_LOG_ERROR_PATH` | `./data/*.log` | 日志 tail 目标 |
| `PANEL_CERT_STORAGE_PATH` | `/data/caddy` | Caddy 证书存储(只读,证书状态查看) |

以上路径/地址类均可在面板"设置"页覆盖(写入 SQLite,下次操作生效);JWT 相关仅 env 管理。

## 工程命令

```bash
# 后端
cd backend
go test ./...                       # 单测
swag init -g cmd/panel/main.go -o docs_swag   # 重新生成 OpenAPI(需 go install github.com/swaggo/swag/cmd/swag@latest)

# 前端
cd frontend
bun run lint        # ESLint + Prettier(提交前必过)
bun run check:antd  # antd 官方 doctor(组件用法/废弃 API)
bun run build       # 类型检查 + 产物写入 backend/internal/web/dist(embed)
```

## 文档

| 文档 | 内容 |
| --- | --- |
| [AGENTS.md](AGENTS.md) | AI 代理/协作者入口:antd 6 查阅姿势、MCP 配置、swaggo 约定、常用命令、提交检查 |
| [需求规格 PRD](docs/requirements.md) | 功能需求与验收标准(编号化)、非功能需求、M2/M3 占位、追溯矩阵 |
| [进度管控](docs/progress.md) | 当前状态、里程碑、任务看板、决策与变更日志 |
| [技术栈选型明细](docs/tech-stack.md) | 各层选型与版本、选型理由、前后端依赖清单、运行形态、工程规范 |
| [项目架构与目录设计](docs/project-structure.md) | 运行时架构、目录结构、关键数据流转、环境变量约定 |
| [M1 详细设计](docs/m1-design.md) | MVP 目标边界、数据模型、API 设计、配置管道、前端页面、任务拆解 |
| [实现方案调研](docs/research-implementation-plan.md) | Caddy 管理能力、竞品对比、架构方案对比与推荐、风险对策 |

## 安全提示

- Caddy Admin API 不要暴露公网;compose 中仅容器网络可达,生产建议 panel 与 Caddy 共享网络命名空间并使用 unix socket。
- 面板为单实例设计(进程内单写者锁),部署固定 replicas=1。

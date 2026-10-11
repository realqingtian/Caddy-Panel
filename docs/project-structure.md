# 项目架构与目录设计

> 状态:随实现演进(M1–M3 交付,2026-10-11 校准至实际结构);本文档描述当前目录结构与各模块职责;演进差异以 [progress 决策日志](./progress.md) 为准。
> 配套阅读:[技术栈明细](./tech-stack.md) · 归档:[M1 详细设计](./archive/m1-design.md)

## 1. 运行时架构

```text
┌─────────────┐  HTTPS + JWT   ┌────────────────────────┐ unix socket / 127.0.0.1:2019 ┌─────────┐
│   浏览器     │ ─────────────► │      Panel 后端 (Go)    │ ────────────────────────────► │  Caddy  │
│ React SPA   │                │  Gin API + 静态资源托管  │   POST /adapt   配置预检        │         │
│ (Antd 6)    │                │  渲染引擎 → 校验 → 下发   │   POST /load    热加载         └────┬────┘
└─────────────┘                │  SQLite(唯一事实源)     │   GET /config   只读读取           │
                               └───────────┬────────────┘   GET /reverse_proxy/upstreams    │
                                           │ 渲染产物落盘 ◄─────────────────────────────────┘
                                           ▼
                                /etc/caddy/Caddyfile(重启恢复保障)

  远程节点 agent(独立 Go module)──反向 WebSocket(心跳/指令/上报)──► Panel :8080 /api/v1/agent/*
```

模块边界与数据流(handler → service → store / caddy 三层):

1. **handler(api 层)**只做协议转换:参数绑定/校验、JWT 解析、统一响应格式,**不写业务**,一律调用 service;
2. **service 层**承载全部业务逻辑:事务编排、审计语义;向下调 **store** 读写数据、调 **caddy 包**执行配置下发;接口先于实现定义,便于 mock 单测;
3. **caddy 包(基础设施)**:**pipeline** 是下发核心编排 —— 领域模型 → **render**(Caddyfile)→ **validate**(离线 adapt 或 `POST /adapt`)→ **下发**(`/load`)→ **落盘** → 版本快照,任一步失败整体失败;**client** 是唯一与 Admin API 通信的出口;
4. **store** 是唯一写库入口(GORM);
5. **agent(独立 Go module,仓库根 `agent/`)**:反向连接中心,自管本机 Caddy(执行 WS 下发指令、上报心跳/漂移/metrics);协议信封定义在中心 `internal/agentproto`,agent 侧持同步副本。

## 2. 目录结构(2026-10-11 实际)

```text
Caddy-Panel/
├── README.md  AGENTS.md  LICENSE
├── docs/                          # 活跃:progress/requirements/tech-stack/project-structure/m4-users-design
│   └── archive/                   # 归档:m1/m2/m3-design、research、completed-milestones、iterations
│
├── agent/                         # 多节点 agent(独立 Go module:caddy-panel/agent)
│   ├── main.go  agent.go          # 反向 WS 连接、心跳(30s)、指令执行、协议握手
│   ├── localcaddy.go              # 本机 Caddy 执行(下发指令调本地 Admin API)
│   └── proto/ …                   # 与中心 internal/agentproto 同步的协议副本
│
├── backend/                       # Go 后端
│   ├── go.mod  go.sum
│   ├── cmd/panel/main.go          # 入口:读配置 → 初始化 store → 启动 API;graceful shutdown
│   ├── templates/                 # Caddyfile 渲染模板(text/template):global / site_proxy / site_static / site_redirect
│   └── internal/
│       ├── config/config.go       # PANEL_* 环境变量(见 §4 环境约定)
│       ├── model/                 # GORM 模型:user site config_version audit_log setting api_token node metrics
│       ├── store/                 # 唯一写库入口:store.go(AutoMigrate)seed.go(首启管理员)
│       ├── caddy/                 # client.go(Admin API 出口)render.go validate.go pipeline.go(单写者+原子落盘)
│       ├── service/               # 全部业务(可 mock 单测):auth/jwt/login_guard、site(+render/validate)、
│       │                          #   config、drift、dashboard、logs、setting、token、backup、cert(+expiry)、
│       │                          #   metrics、smtp、importer、node(+heartbeat/install)
│       ├── api/
│       │   ├── router.go          # 路由 + 中间件挂载 + service 装配;/healthz
│       │   ├── middleware/        # auth(JWT) audit(唯一写入方) cors recover limiter(登录限速)
│       │   ├── handler/           # 协议转换:auth/sites/config/logs/settings/tokens/backup/cert/metrics/
│       │   │                      #   upstreams/node/terminal/import/dashboard/agent_ws/agent_report
│       │   └── agent_ws.go 等     # agent 反向 WS hub + 上报接收
│       ├── agentconn/  agentproto/# agent 连接管理与协议信封(与 agent/ 侧同步)
│       ├── apierr/  i18n/  version/ # 统一错误 {code,message,detail}、后端消息中英、版本号
│       └── web/                   # go:embed 前端产物(生产单二进制)
│
├── frontend/                      # React 19 + Vite + TS + Antd 6(依赖管理:Bun)
│   ├── package.json  bun.lock  vite.config.ts(/api proxy → :8080)
│   ├── scripts/check-i18n.mjs     # i18n 静态审查(迭代 I1 沉淀,发版前执行)
│   └── src/
│       ├── main.tsx               # QueryClientProvider + ConfigProvider(antd locale)+ RouterProvider
│       ├── router.tsx  index.css  # 路由 + 登录守卫;全局样式(登录页/布局壳/终端)
│       ├── api/                   # axios client + 按域拆分(auth/sites/config/logs/nodes/metrics/tokens/backup/upstreams/dashboard)+ types.ts
│       ├── stores/                # zustand:auth.ts(token/用户/角色)、theme.ts(语言/主题,persist)
│       ├── locales/               # zh-CN.json、en-US.json、index.ts(react-i18next)
│       ├── hooks/  components/    # usePolling 等;BrandMark、SiteForm、CaddyfileEditor 等
│       ├── layouts/AdminLayout.tsx# 应用壳:Sider(固定)+ Header(页面标题/主题/语言/用户)+ 内容区独立滚动
│       └── pages/                 # Login Dashboard Sites Upstreams Nodes ConfigEditor Logs Metrics Settings Terminal
│
└── deploy/
    ├── docker-compose.yml         # panel + caddy(含插件镜像)两服务
    ├── panel.Dockerfile           # 多阶段:bun build 前端 → swag → go build 单二进制
    └── caddy/                     # Caddy 引导配置(admin :2019、日志落文件)
```

**演进说明:**

- M2/M3 扩展点(上游子表、监控、导入、终端、多节点)均已落地,落地记录见 [archive/completed-milestones.md](./archive/completed-milestones.md);
- **M4 用户系统扩展点**:`users` 加列(disabled/totp_*)、新增 `sessions` 表、权限点矩阵替换两级 role 判断,设计见 [m4-users-design.md](./m4-users-design.md);
- **版本对齐**:Panel 内嵌的离线适配器(import 的 caddy 库版本)须与部署的 Caddy 保持同一 minor 序列(compose 中两者同步升级),避免"离线校验通过、`/load` 被运行时拒绝"的版本偏差。

## 3. 关键流转(以"新建反代站点"为例)

1. 前端 `Sites` 页提交表单 → `POST /api/v1/sites`;
2. `handler/sites.go` 绑定参数 → `service/site.go` 编排业务:
   a. 落库 `sites`(enabled);
   b. 调 `caddy/pipeline`:render.go 用 `site_proxy.tmpl` 重新渲染**全量** Caddyfile → validate.go 校验,失败 → 返回 422 + Caddy 错误原文并回滚 → client.Load(json) 热加载(Caddy 失败自动回滚旧配置);
   c. Caddyfile 原子写盘(临时文件 + rename)+ `config_versions` 插入快照(status=applied);请求级审计由 audit 中间件统一记录(service 不重复写);
3. 前端 TanStack Query 失效 `sites`/`versions` 缓存,列表与版本历史自动刷新。

### 3.1 并发与失败语义(设计关键点)

- **单写者**:`caddy/pipeline` 持全局写锁,所有下发(新建/更新/删除/回滚/Caddyfile 直编)串行执行 —— 全量渲染模式下并发写会互相覆盖,串行化是正确性前提;
- **失败顺序**:校验失败 → DB 事务回滚、不下发;`/load` 失败(Caddy 侧已自动回滚旧配置)→ 回滚 DB 变更并返回错误 —— 任何时刻 DB、磁盘 Caddyfile、Caddy 运行态三者一致;
- **双保险**:Panel 内单写者之外,下发携带 `If-Match`(取自 `GET /config` 的 Etag),防面板外工具并发改动;
- **优雅停机**:收到退出信号后停止接收请求,等待进行中的下发完成再退出,杜绝中间态。

全量渲染(而非增量 PATCH)是刻意取舍:领域模型是唯一事实源,渲染结果可预期、可 diff、可回滚,避开 `/config/...` 细粒度路径操作的复杂度;Caddy 端热加载本身零停机,全量替换没有性能问题。

## 4. 环境约定

| 变量 | 默认(开发) | 说明 |
| --- | --- | --- |
| `PANEL_LISTEN` | `:8080` | Panel API 监听 |
| `PANEL_MODE` | `debug` | Gin 运行模式(`release` 用于生产/镜像) |
| `PANEL_DB_PATH` | `./data/panel.db` | SQLite 路径 |
| `PANEL_ADMIN_URL` | `http://localhost:2019` | Caddy Admin API;生产推荐 `unix:///run/caddy/admin.sock`(client 以 unix socket dial) |
| `PANEL_CADDY_ADMIN_BIND` | `localhost:2019` | 渲染全局块的 Caddy admin 监听地址(Caddy 重启恢复依赖此绑定);compose 中为 `caddy:2019` |
| `PANEL_CADDYFILE_PATH` | `./data/Caddyfile`(生产 `/etc/caddy/Caddyfile`) | 渲染产物落盘位置 |
| `PANEL_JWT_SECRET` | 启动时生成并持久化到 settings | JWT 签名密钥 |
| `PANEL_JWT_TTL` | `24h` | JWT 有效期;仅 .env 配置,重启生效(不进设置页) |
| `PANEL_LOGIN_MAX_FAILS` | `5` | 登录连续失败锁定阈值;设置页可改,立即生效 |
| `PANEL_LOGIN_LOCK_WINDOW` | `1m` | 登录锁定时长;设置页可改,立即生效 |
| `PANEL_LOG_ACCESS_PATH` / `PANEL_LOG_ERROR_PATH` | `./data/access.log` / `./data/error.log` | 日志 tail 读取目标 |
| `PANEL_CERT_STORAGE_PATH` | `/data/caddy` | Caddy 证书存储根目录(只读展示,FR-308;compose 中共享 caddy_data 卷) |
| `PANEL_PANEL_PUBLIC_URL` | 空 | 面板对外可达地址(agent 安装命令生成 ws URL 用);可在设置页覆盖 |

> 优先级:env 提供**默认值**;路径类与登录锁定参数可在设置页(FR-601)修改并持久化到 `settings` 表,运行时以 settings 为准;`PANEL_JWT_TTL` 与 `PANEL_JWT_SECRET` 仅 env/启动侧管理。

## 5. 测试与质量约定

- 各包 `_test.go` 与源码同目录(Go 惯例),不设独立 test 目录;
- **service 层**:依赖全部走接口(mock store 与 caddy.Client),重点覆盖失败分支(校验失败回滚、load 失败回滚);
- **handler 层**:httptest + Gin 起真路由,验证鉴权、参数校验、统一错误格式;
- **caddy/pipeline**:render 单测用快照对比(golden file);validate/load 为集成测试,标记 build tag `integration`,依赖本机 Caddy,CI 缺省跳过;
- **前端**:Vitest 覆盖 api 层与关键组件;`bun run lint`(ESLint 10 flat config + Prettier)提交前必过;`bun run check:antd`(antd 官方 doctor)检查组件用法与废弃 API。

# 项目架构与目录设计

> 状态:设计定稿(2026-10-10),尚未初始化代码。本文档描述**将要创建**的完整目录结构与各模块职责。
> 配套阅读:[技术栈明细](./tech-stack.md) · [M1 详细设计](./m1-design.md)

## 1. 运行时架构

```
┌─────────────┐  HTTPS + JWT   ┌────────────────────────┐ unix socket / 127.0.0.1:2019 ┌─────────┐
│   浏览器     │ ─────────────► │      Panel 后端 (Go)    │ ────────────────────────────► │  Caddy  │
│ React SPA   │                │  Gin API + 静态资源托管  │   POST /adapt   配置预检        │         │
│ (Antd 6)    │                │  渲染引擎 → 校验 → 下发   │   POST /load    热加载         └────┬────┘
└─────────────┘                │  SQLite(唯一事实源)     │   GET /config   只读读取           │
                               └───────────┬────────────┘   GET /reverse_proxy/upstreams    │
                                           │ 渲染产物落盘 ◄─────────────────────────────────┘
                                           ▼
                                /etc/caddy/Caddyfile(重启恢复保障)
```

模块边界与数据流(handler → service → store / caddy 三层):

1. **handler(api 层)**只做协议转换:参数绑定/校验、JWT 解析、统一响应格式,**不写业务**,一律调用 service;
2. **service 层**承载全部业务逻辑:事务编排、审计语义;向下调 **store** 读写数据、调 **caddy 包**执行配置下发;接口先于实现定义,便于 mock 单测;
3. **caddy 包(基础设施)**:**pipeline** 是下发核心编排 —— 领域模型 → **render**(Caddyfile)→ **validate**(离线 adapt 或 `POST /adapt`)→ **下发**(`/load`)→ **落盘** → 版本快照,任一步失败整体失败;**client** 是唯一与 Admin API 通信的出口;
4. **store** 是唯一写库入口(GORM)。
4. 前端经 `/api/v1` 访问,生产模式由 Go 用 `go:embed` 直接托管前端产物,开发模式由 Vite proxy。

## 2. 目录结构(规划)

```
Caddy-Panel/
├── README.md
├── docs/                          # 全部设计文档(已完成 4 篇)
│
├── backend/                       # Go 后端
│   ├── go.mod                     # module: caddy-panel/backend
│   ├── cmd/panel/main.go          # 入口:读配置 → 初始化 store → 启动 API;graceful shutdown(等待进行中的下发完成)
│   ├── templates/                 # Caddyfile 渲染模板(text/template)
│   │   ├── global.tmpl            #   全局块:admin 绑定、log 输出、可选 metrics
│   │   ├── site_proxy.tmpl        #   反代站点块
│   │   ├── site_static.tmpl       #   静态站点块
│   │   └── site_redirect.tmpl     #   重定向站点块
│   └── internal/
│       ├── config/config.go       # 环境变量/启动参数:监听地址、Caddy admin 地址、Caddyfile 路径、JWT 密钥
│       ├── model/                 # GORM 模型(与 M1 设计第 3 节数据模型一一对应)
│       │   ├── user.go  site.go  config_version.go  audit_log.go  setting.go
│       ├── store/
│       │   ├── store.go           # 打开 SQLite、AutoMigrate(M1;后续引入 goose/golang-migrate 版本化迁移)、事务助手
│       │   └── seed.go            # 首次启动创建管理员(用户名默认 admin):密码取 PANEL_ADMIN_PASSWORD,未设置则随机生成并打印到日志(仅一次)
│       ├── caddy/
│       │   ├── client.go          # Admin API client:Load/Adapt/GetConfig/Upstreams/Stop
│       │   ├── render.go          # 模板渲染引擎:领域字段 → Caddyfile,含 raw_extra/global_extra 片段原样拼入
│       │   ├── validate.go        # 校验:优先进程内 httpcaddyfile 适配,兜底 POST /adapt
│       │   └── pipeline.go        # 编排:render → validate → load → 原子落盘(tmp+rename)→ 留版本快照;内置全局单写者锁,串行化所有下发
│       ├── service/               # 业务逻辑层(全部业务在这里,可单测:mock store 与 caddy 接口)
│       │   ├── auth.go           # 登录、JWT 签发、bcrypt 密码校验
│       │   ├── site.go           # 站点 CRUD 业务:落库 → 触发下发管道(结果记入 config_versions)
│       │   ├── config.go         # 版本查询/对比、回滚、Caddyfile 直接编辑
│       │   ├── drift.go          # 漂移检测:DB 快照 / 磁盘 Caddyfile / GET /config 三方对比,启动与 Dashboard 触发
│       │   ├── dashboard.go      # 状态聚合(Caddy 存活/版本/站点数/最近下发)
│       │   └── logs.go           # 日志 tail 读取与轮询
│       └── api/
│           ├── router.go          # 路由注册 + 中间件挂载 + service 装配;/healthz 健康端点(供容器探活)
│           ├── middleware/        # auth.go(JWT) audit.go(请求级审计:谁/何时/哪个API/结果码,唯一写入方) cors.go(gin-contrib) recover.go limiter.go(登录接口内存限速)
│           └── handler/           # auth.go status.go sites.go config.go logs.go(仅参数绑定/校验/统一响应,不写业务)
│
├── frontend/                      # React 19 + Vite + TS + Antd 6(依赖管理:Bun)
│   ├── package.json               # scripts: dev/build/lint(bun run ...)
│   ├── bun.lock                   # Bun 锁文件(提交入库)
│   ├── vite.config.ts             # react 插件;/api proxy → http://localhost:8080
│   ├── tsconfig.json  index.html
│   └── src/
│       ├── main.tsx               # QueryClientProvider + RouterProvider + AntdConfigProvider(zh_CN)
│       ├── router.tsx             # 路由表 + 登录守卫(未登录重定向 /login)
│       ├── api/
│       │   ├── client.ts          # axios 实例:baseURL、JWT 拦截器、401 跳登录、错误归一化
│       │   ├── types.ts           # 与后端契约对应的 TS 类型(Site、ConfigVersion、Status...)
│       │   └── auth.ts  sites.ts  config.ts  logs.ts
│       ├── stores/                # Zustand:auth.ts(token/当前用户)、theme.ts(语言/主题偏好)
│       ├── locales/               # i18n 文案:zh-CN.json、en-US.json、index.ts(react-i18next 初始化)
│       ├── hooks/                 # 通用 hooks(usePolling 日志轮询、useAuth)
│       ├── layouts/AdminLayout.tsx# Sider 菜单 + Header(用户菜单/退出)
│       ├── pages/
│       │   ├── Login/             # 登录表单
│       │   ├── Dashboard/         # 状态卡片 + 最近下发记录
│       │   ├── Sites/             # 列表 Table + SiteForm Drawer(按 type 动态字段)
│       │   ├── ConfigEditor/      # Monaco 编辑 + 版本历史 + DiffViewer + 回滚
│       │   ├── Logs/              # access/error Tab + 轮询 tail
│       │   └── Settings/          # M1 仅展示与修改基础设置
│       └── components/            # SiteForm、CaddyfileEditor(Monaco+Caddyfile 高亮)、VersionDiff
│
├── deploy/
│   ├── docker-compose.yml         # caddy + panel 两服务;panel 依赖 caddy 健康
│   ├── panel.Dockerfile           # 多阶段:bun build 前端 → go build 单二进制
│   └── caddy/Caddyfile.initial    # Caddy 引导配置(admin :2019、日志输出到文件)
│
└── scripts/dev.sh                 # 一键起本地开发(可选)
```

**M2/M3 扩展点(先留边界、不建目录,演进路径已定):**

- **多节点 Agent(M3)**:独立 `agent/` 模块 + 中心下发协议;`caddy.Client`/`pipeline` 方法签名不写死 host,多节点时按节点实例化;届时 `sites`/`config_versions` 增加 `node_id` 列(AutoMigrate + 一次性回填 `local`),API 按节点过滤;
- **upstream 独立配置(M2)**:`upstreams` 当前为逗号分隔 TEXT;M2 引入健康检查/权重时拆 `site_upstreams` 子表(AutoMigrate + 一次性回填),渲染产物不变;
- **监控(M2)**:全局模板已预留 metrics 位,开关在设置页(FR-601),M2 只需补抓取与图表;
- **导入与复杂配置(M2)**:`sites.raw_extra` / `settings.global_extra` 片段通道 M1 已预留,M2 导入 Caddyfile 时不可识别内容落入片段,渲染管道零改动;
- **Web 终端(M3)**:后端 WebSocket handler + xterm.js;
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
| `PANEL_DB_PATH` | `./data/panel.db` | SQLite 路径 |
| `PANEL_ADMIN_URL` | `http://localhost:2019` | Caddy Admin API;生产推荐 `unix:///run/caddy/admin.sock`(client 以 unix socket dial) |
| `PANEL_CADDY_ADMIN_BIND` | `localhost:2019` | 渲染全局块的 Caddy admin 监听地址(Caddy 重启恢复依赖此绑定);compose 中为 `caddy:2019` |
| `PANEL_CADDYFILE_PATH` | `./data/Caddyfile`(生产 `/etc/caddy/Caddyfile`) | 渲染产物落盘位置 |
| `PANEL_JWT_SECRET` | 启动时生成并持久化到 settings | JWT 签名密钥 |
| `PANEL_JWT_TTL` | `24h` | JWT 有效期;仅 .env 配置,重启生效(不进设置页) |
| `PANEL_LOGIN_MAX_FAILS` | `5` | 登录连续失败锁定阈值;设置页可改,立即生效 |
| `PANEL_LOGIN_LOCK_WINDOW` | `1m` | 登录锁定时长;设置页可改,立即生效 |
| `PANEL_LOG_ACCESS_PATH` / `PANEL_LOG_ERROR_PATH` | `./data/access.log` / `./data/error.log` | 日志 tail 读取目标 |

> 优先级:env 提供**默认值**;路径类与登录锁定参数可在设置页(FR-601)修改并持久化到 `settings` 表,运行时以 settings 为准;`PANEL_JWT_TTL` 与 `PANEL_JWT_SECRET` 仅 env/启动侧管理。

## 5. 测试与质量约定

- 各包 `_test.go` 与源码同目录(Go 惯例),不设独立 test 目录;
- **service 层**:依赖全部走接口(mock store 与 caddy.Client),重点覆盖失败分支(校验失败回滚、load 失败回滚);
- **handler 层**:httptest + Gin 起真路由,验证鉴权、参数校验、统一错误格式;
- **caddy/pipeline**:render 单测用快照对比(golden file);validate/load 为集成测试,标记 build tag `integration`,依赖本机 Caddy,CI 缺省跳过;
- **前端**:Vitest 覆盖 api 层与关键组件;`bun run lint`(ESLint 10 flat config + Prettier)提交前必过;`bun run check:antd`(antd 官方 doctor)检查组件用法与废弃 API。

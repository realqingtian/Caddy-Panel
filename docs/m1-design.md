# Caddy Panel — M1(MVP)详细设计

> 前置文档:[实现方案调研](./research-implementation-plan.md) · [技术栈明细](./tech-stack.md) · [项目架构与目录设计](./project-structure.md)
> 技术栈:Go 后端 + React 前端 + SQLite(前端依赖管理:Bun)

## 1. 目标与非目标

**M1 目标(单机可用)**

- 登录鉴权(单管理员账号,JWT)
- Dashboard:Caddy 运行状态、版本、已加载站点数
- 站点管理(表单化):反向代理、静态站点、重定向
- 配置中心:Caddyfile 预览/编辑、保存即校验(adapt)→ 热加载(/load)、失败回滚提示
- 配置版本历史 + diff + 一键回滚
- 日志在线查看(访问日志/错误日志 tail)

**非目标(M1 不做)**

- 多节点/Agent、集群、RBAC 多角色
- Web 终端、插件管理、四层转发
- 监控图表(/metrics 接入,仅预留 API)
- 证书手动上传/DNS challenge 配置 UI(自动 HTTPS 默认开启即可用)

## 2. 总体架构

```
┌─────────────┐   HTTPS/JWT   ┌──────────────────┐  unix socket / 127.0.0.1:2019  ┌─────────┐
│   浏览器     │ ───────────► │  Panel(Go 后端)  │ ──────────────────────────────► │  Caddy  │
│ React + Antd │              │  SQLite + 渲染引擎 │      POST /adapt  预检          │         │
└─────────────┘              │  (唯一事实源: DB)  │      POST /load   热加载        └─────────┘
                             └────────┬─────────┘      GET  /config  只读展示      │
                                      │ 渲染 Caddyfile 落盘 ◄──────────────────── ┘
                                      ▼
                             /etc/caddy/Caddyfile(重启恢复保障)
```

- Panel 是 Admin API 的唯一入口与鉴权层;Caddy Admin API 仅绑定 localhost。
- Panel DB 为唯一事实源;Caddyfile 是渲染产物 + 重启保障。
- 所有下发走"渲染 → 校验 → load"管道,任何一步失败都不落库、不生效。

## 3. 数据模型(SQLite)

```sql
users             -- M1 单管理员,表结构预留角色字段
  id, username, password_hash(bcrypt), role, created_at

sites             -- 站点(领域模型核心表)
  id, name,                       -- 展示名
  type,                           -- proxy | static | redirect
  domains,                        -- TEXT,逗号分隔,如 "a.com, www.a.com"
  -- proxy 专用
  upstreams,                      -- TEXT,逗号分隔 "127.0.0.1:8080, 127.0.0.1:8081"
  lb_policy,                      -- round_robin(默认)| least_conn | ip_hash | first
  websocket,                      -- BOOL(渲染 flush_interval -1 与 Upgrade 透传)
  headers_enabled, custom_headers -- JSON [{"key","value"}](响应头)
  -- static 专用
  root_path,                      -- /var/www/a.com
  -- redirect 专用
  redirect_to, redirect_permanent -- BOOL(301/308 vs 302)
  https_redirect,                 -- BOOL,http→https(默认 true)
  enabled,                        -- BOOL,停用站点不渲染
  created_at, updated_at

config_versions   -- 每次成功/失败下发都留痕
  id, sites_snapshot(JSON), rendered_caddyfile(TEXT), status(applied|failed),
  error_msg, created_at

audit_logs        -- 操作审计
  id, user_id, action, target, detail(JSON), ip, created_at

settings          -- KV:日志路径、Caddyfile 路径、admin endpoint 等
  key, value
```

## 4. 配置管道(核心流程)

1. **渲染**:`sites` 表 → Go 模板 → Caddyfile(全局块:admin 绑定、log 输出、可选 metrics;每站点一个站点块)。
2. **预检**(二选一,优先前者):
   - 离线:import `github.com/caddyserver/caddy/v2/caddyconfig/caddyfile` + `httpcaddyfile` 适配器,Caddyfile → JSON,不触网;
   - 在线兜底:`POST /adapt`(Content-Type: `text/caddyfile`)。
3. **下发**:`POST /load`(JSON,`Content-Type: application/json`),Caddy 零停机热加载;失败 Caddy 自动回滚,Panel 返回错误并标记 `config_versions.status=failed`。
4. **落盘**:渲染的 Caddyfile 写入目标路径(默认 `/etc/caddy/Caddyfile`,本机开发可配),**原子写**(临时文件 + rename)。写成功后才更新 `config_versions.status=applied`。
5. **启动漂移检测**:Panel 启动时对比 DB 最新成功版本与磁盘 Caddyfile + `GET /config`,不一致则在 Dashboard 告警(防"面板外手改/重启回退")。
6. **并发与失败语义**:pipeline 内置全局单写者锁,所有下发串行执行(全量渲染下并发写会互相覆盖);校验失败 → DB 事务回滚不下发;`/load` 失败 → 回滚 DB(Caddy 侧已自动回滚),保证 DB / Caddyfile / 运行态三者一致;下发携带 `If-Match` 防面板外并发改动。

## 5. 后端 API 设计(REST,`/api/v1`)

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/auth/login` | 登录,返回 JWT |
| GET | `/me` | 当前用户 |
| GET | `/status` | Dashboard:Caddy 存活、版本、站点数、最近下发 |
| GET/POST | `/sites` | 站点列表 / 新建(新建即渲染+校验+下发,可传 `dry_run`) |
| GET/PUT/DELETE | `/sites/{id}` | 详情 / 更新 / 删除 |
| POST | `/sites/{id}/enable`、`/disable` | 启停 |
| GET | `/config/current` | `GET /config` 透传(只读展示当前生效 JSON) |
| GET | `/config/versions` | 版本历史列表 |
| GET | `/config/versions/{id}` | 版本详情(含 Caddyfile 全文,供前端 diff) |
| POST | `/config/versions/{id}/rollback` | 回滚:取快照 → 校验 → load |
| POST | `/config/caddyfile` | 直接编辑 Caddyfile:校验通过才落盘生效(进阶模式) |
| GET/PUT | `/settings` | 系统设置 KV(Caddyfile 路径、日志路径、Admin API 地址);env 提供默认值,此处修改持久化覆盖 |
| GET | `/logs?type=access\|error&tail=200` | 日志 tail(轮询,M1 不做 WebSocket) |

错误约定:统一 `{ "code": "...", "message": "...", "detail": ... }`;校验失败返回 422 + Caddy 错误原文。全部接口编写 swaggo 注解,`swag init` 生成 OpenAPI,开发态 Swagger UI 挂载在 `/swagger`。

## 6. 前端设计(React 19 + TypeScript + Ant Design 6)

- 构建:Vite;**依赖与脚本管理:Bun**(`bun install` / `bun run dev`,本机已装 bun 1.4.2);路由:React Router;服务端状态:TanStack Query;本地状态:Zustand(登录态/主题);请求:axios(拦截器统一带 JWT、401 跳登录)。
- 布局:Sider 菜单(仪表盘/站点管理/配置中心/日志/设置)+ Content;Header 放语言切换(中/英)与主题切换(亮/暗/跟随系统);
- **国际化**:react-i18next,文案集中在 `src/locales/{zh-CN,en-US}.json`,**禁止硬编码 UI 文案**;antd 组件库文案经 ConfigProvider locale 同步切换,语言偏好持久化;
- **主题**:antd 6 `theme.algorithm`(亮/暗)三态切换(跟随系统 `prefers-color-scheme`),偏好持久化;
- 页面:
  - **Login**:居中卡片表单;
  - **Dashboard**:状态卡片(Caddy 存活/版本/站点数)+ 最近下发记录;
  - **Sites**:Table + 新建/编辑 Drawer(按 type 动态字段),启停 Switch,删除 Popconfirm;
  - **ConfigEditor**:左"渲染预览/直接编辑" Monaco(只读渲染产物 + 可编辑进阶模式),右版本历史 + diff( monaco diff editor)+ 回滚按钮;
  - **Logs**:Tab(access/error)+ 自动轮询 tail。
- Caddyfile 语法高亮:Monaco 注册自定义 language(monarch tokenizer,关键词来自指令表,轻量实现)。

## 7. 工程结构(monorepo)

```
Caddy-Panel/
├── backend/                 # Go
│   ├── cmd/panel/main.go
│   └── internal/
│       ├── api/             # 路由 + handler(仅协议转换)+ 中间件(JWT/CORS/审计)
│       ├── service/         # 业务逻辑层(站点/配置下发/回滚/日志/状态聚合)
│       ├── caddy/           # Admin API client、adapt/load、Caddyfile 渲染(基础设施)
│       ├── model/           # GORM 模型
│       └── store/           # SQLite 初始化、迁移
├── frontend/                # React 19 + Vite + TS + Antd 6(Bun 管理依赖)
│   └── src/{api,pages,layouts,components,stores}
├── deploy/
│   ├── docker-compose.yml   # caddy + panel
│   └── Caddyfile            # 初始引导配置
└── docs/
```

- 开发:后端 `:8080`,前端 Vite `:5173`(proxy `/api` → `:8080`),本机 Caddy `:2019`。
- 交付:Docker Compose(panel + caddy 两容器,共享网络)与 Go 单二进制双形态。

## 8. 任务拆解(按顺序)

> 任务**状态**统一在 [进度看板](./progress.md) 维护,此处只保留定义;需求验收标准见 [PRD](./requirements.md)。

1. 脚手架:前后端骨架可运行、compose 可起;接入 antd 官方 AI 协作资源(MCP/Skill,配置见 AGENTS.md)
2. 登录/JWT + SQLite 迁移 + 审计/限速中间件
3. Caddy Admin client(/adapt、/load、/config、/status)
4. Caddyfile 渲染引擎(站点模板 + 全局模板)+ 离线校验 + pipeline 编排(单写者/原子落盘)
5. 站点 CRUD API + 前端 Sites 页
6. 配置中心(版本/回滚/diff/系统设置)+ ConfigEditor/Settings 页
7. 日志 tail API + Logs 页
8. 启动漂移检测 + Dashboard
9. Docker 镜像与 compose 打磨、README

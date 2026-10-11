# Caddy 可视化管理系统(Caddy Panel)实现方案调研

> 调研日期:2026-10-10

## 1. 结论(TL;DR)

- **Caddy 自带官方 Admin API**(默认 `localhost:2019`),支持运行时读/写配置、优雅零停机重载、失败自动回滚。这是面板最可靠的控制通道,**不需要侵入或重新编译 Caddy**。
- **推荐架构:独立面板进程 + 领域模型(DB)为唯一事实源的混合管道** —— DB 中的站点/反代等领域对象 → 模板渲染 Caddyfile(落盘,保证重启恢复)→ 离线 adapt 校验 → `POST /load`(JSON)热加载生效。
- **推荐技术栈:Go 后端 + React 前端(依赖管理用 Bun)+ SQLite,单二进制交付**。Go 可直接复用 Caddy 官方库做 Caddyfile 解析/格式化/适配;Go 单二进制模式已被 Nginx UI(11.6k stars)验证受欢迎。
- **现有开源 Caddy 面板均不成熟**(Alpha/实验阶段、低 star、功能单一),功能对标 Nginx Proxy Manager 的易用性 + Nginx UI 的完成度,差异化空间大。

## 2. Caddy 提供的管理能力(面板的技术基础)

### 2.1 Admin API(默认 `localhost:2019`)

| 端点 | 方法 | 用途 |
| --- | --- | --- |
| `/load` | POST | 整体替换配置;阻塞直到完成;失败自动回滚旧配置,**零停机** |
| `/stop` | POST | 优雅关闭进程 |
| `/config/[path]` | GET/POST/PUT/PATCH/DELETE | 按 JSON 路径细粒度读写配置(如 `/config/apps/http/servers/srv1/listen`) |
| `/id/<id>[...]` | any | 通过 JSON 中的 `@id` 字段快捷寻址,免写长路径 |
| `/adapt` | POST | Caddyfile → JSON 适配但**不生效**(Content-Type: `text/caddyfile`)—— 面板做配置预检/验证的关键 |
| `/pki/ca/<id>`、`/pki/ca/<id>/certificates` | GET | 内部 CA 信息与证书链 |
| `/reverse_proxy/upstreams` | GET | 反代上游健康状态(address / 请求数 / 失败数) |
| `/metrics` | GET | Prometheus 指标(需在配置中启用 `metrics` 指令) |

关键行为:

- **乐观并发控制**:`GET /config/...` 返回 `Etag`,变更请求带 `If-Match` 头,冲突返回 `412`。面板必须实现,避免多端并发写互相覆盖。
- **无内置认证机制**:官方建议绑定 unix socket 或仅 localhost。远程暴露需自行加 TLS/防火墙/授权 → **面板自身必须提供登录鉴权,作为 Admin API 的唯一暴露入口**。
- Content-Type 可选格式:`application/json`(原生)、`text/caddyfile`、`application/json5` 等。

### 2.2 配置持久化(重要坑)

- 通过 API 修改的配置会自动持久化到 `autosave.json`(`~/.config/caddy/`,systemd 安装在 `/var/lib/caddy/.config/caddy/`),启动时用 `caddy run --resume` 恢复。
- **但** systemd 默认安装是 `caddy run --config /etc/caddy/Caddyfile`,重启会回到 Caddyfile,**API 的改动全部丢失**。
- 结论:**面板必须同时维护 Caddyfile 落盘**(或指导用户改为 `caddy-api` 包 / `--resume` 启动),否则"重启丢配置"是必然事故。

### 2.3 官方 Go 库可复用(免自研解析器)

- `github.com/caddyserver/caddy/v2/caddyconfig/caddyfile`:Caddyfile 解析、格式化(即 `caddy fmt` 的逻辑)。
- `caddyconfig` 适配器:Caddyfile → JSON(与 `/adapt` 等价),可**离线**执行,不依赖运行中的 Caddy。
- 面板的离线校验、语法高亮、diff、导入导出都因此可做,成本大幅降低。

## 3. 现有方案对比

| 项目 | 形态 | 技术栈 | 管理方式 | 成熟度 |
| --- | --- | --- | --- | --- |
| [gsmlg-dev/caddy-admin-ui](https://github.com/gsmlg-dev/caddy-admin-ui) | Caddy 插件(需 xcaddy 自编译) | Go + 内嵌 UI | 代理 Admin API | 实验性,58 stars |
| [caddymanager/caddymanager](https://github.com/caddymanager/caddymanager) | 独立面板,多机管理 | MEVN(Vue3+Express+Mongo/SQLite) | — | Alpha(2025-06 发布) |
| [rhad00/CaddyManager](https://github.com/rhad00/CaddyManager) | 单机域名反代 UI | — | Caddyfile | 小型 |
| [lyarinet/caddyserver-manager](https://github.com/lyarinet/caddyserver-manager) | systemd 服务 | — | — | 小型 |

参考标杆(非 Caddy 但定义了用户预期):

- **Nginx Proxy Manager**:易用性标杆。Docker 一键部署、反代主机表单化配置、Let's Encrypt 一键签发(HTTP/DNS challenge)、访问控制列表、TCP/UDP 流转发、多用户。
- **[Nginx UI](https://github.com/0xJacky/nginx-ui)(~11.6k stars)**:完成度标杆。单二进制(Go+Vue)、配置文件式管理 + 保存即校验重载、自动备份/版本 diff/回滚、证书自动续期、CPU/内存/磁盘监控、在线日志、WebSocket 终端、多节点集群、MCP/AI 助手。

**共同缺口**:目前没有一个完成度高的面板是"Caddy 官方 API 优先 + 兼容 Caddyfile 生态"的 —— 这正是本项目的定位空间。

## 4. 架构方案对比与推荐

### 方案 A:纯 Admin API 动态配置

面板直接以 JSON 配置为中心,改动经 `/load` 或 `/config/...` 热更。

- 优点:官方通道、优雅重载、细粒度修改、失败自动回滚。
- 缺点:原生 JSON 极其冗长;重启恢复依赖 `--resume`;与 Caddyfile 手写生态割裂。

### 方案 B:管理 Caddyfile 文件 + reload(Nginx UI 模式)

面板编辑 Caddyfile 文件,保存后调用 `caddy reload`。

- 优点:格式人类友好、git 友好、与现有部署完全兼容。
- 缺点:需要面板与 Caddy 同机且有 exec 权限;从 Caddyfile 反向解析出结构化模型困难。

### 方案 C(推荐):领域模型 + 混合管道

```
用户操作(UI 表单)
      ↓
领域模型(SQLite:站点/反代/证书/规则)
      ↓ 渲染
Caddyfile 模板 ──落盘──► /etc/caddy/Caddyfile(重启恢复保障 + git 版本化)
      ↓ 离线 adapt 校验(官方 Go 库或 POST /adapt)
   校验失败 → 提示并回滚,不下发
      ↓ POST /load(JSON)
Caddy 热加载生效(零停机,失败自动回滚)
```

- 面板 DB 是唯一事实源;Caddyfile 是"渲染产物 + 重启保障",同时保持用户可用手工格式查看。
- 当前生效配置通过 `GET /config` 只读展示,避免反向建模负担。
- "导入现有 Caddyfile"作为迁移功能:可识别的部分映射为领域模型,识别不了的保留为"自定义片段"原样输出。

### 部署拓扑

- **单机版(MVP)**:面板与 Caddy 同机,通过 **unix socket**(推荐)或 `127.0.0.1:2019` 通信。
- **多节点版(后续)**:每节点部署轻量 Agent(本质是面板的受控代理),中心面板统一下发配置、收集状态 —— CaddyManager 即此路线,但实现不成熟,可作为差异化方向。

### 通信安全

- 面板 ↔ Caddy:unix socket 或 localhost,严禁裸暴露 Admin API。
- 用户 ↔ 面板:HTTPS + 登录鉴权(用户/角色/审计日志),面板是 Admin API 的唯一入口。

## 5. 推荐技术栈

| 层 | 选型 | 理由 |
| --- | --- | --- |
| 后端 | Go + Gin + GORM + SQLite | 单二进制跨平台分发(Nginx UI 验证过该模式受欢迎);Gin 为 Go 生态最主流 Web 框架;可直接 import Caddy 官方库做 adapt/parse/fmt;与 Caddy 部署形态契合 |
| 前端 | React 19 + TypeScript + Vite + Ant Design 6;依赖管理用 **Bun** | 组件库成熟、中后台组件覆盖全(antd 6 原生支持 React 19);配套 React Router、TanStack Query(服务端状态)、Zustand(本地状态);Bun 安装/脚本执行显著快于 npm |
| 编辑器 | Monaco(@monaco-editor/react,带 Caddyfile 高亮) | 配置编辑体验,VS Code 同源内核 |
| 存储 | SQLite(单机零依赖);预留 MySQL/Postgres(多节点中心版) | 部署简单 |
| 实时能力 | WebSocket(日志实时流、终端) | 对标 Nginx UI |
| 交付 | Docker 镜像(面板 + Caddy sidecar)与单二进制双形态 | 覆盖自托管两类人群 |

## 6. 功能规划(分期)

### M1 — MVP(单机可用)

- 登录鉴权、Dashboard(Caddy 版本、运行状态、uptime)
- 站点管理表单:反向代理(域名、目标 upstream、WebSocket 支持、自定义 header)、静态站点、重定向
- HTTPS:证书状态展示(Caddy 自动 HTTPS 是默认行为)、强制跳转
- 配置中心:Caddyfile 预览/编辑,保存即 adapt 校验 → 热加载,失败回滚提示
- 版本历史 + diff + 一键回滚(DB 快照或 git)
- 日志:访问日志/错误日志在线查看(Caddy log 输出到文件,面板 tail)

### M2 — 进阶

- 负载均衡策略与健康检查可视化(对接 `/reverse_proxy/upstreams`)
- 访问控制:Basic Auth、IP 白名单、限流(限流为插件,需标注依赖)
- 监控:接入 `/metrics`,内置图表或一键对接 Prometheus + Grafana
- 证书:ACME 账号管理、DNS challenge 各供应商配置、手动上传证书
- 配置备份/恢复、导入现有 Caddyfile

### M3 — 平台化

- 多节点 Agent 模式(集中下发、集中 Dashboard)
- Web 终端、插件管理(xcaddy 自定义构建指引)
- TCP/UDP 四层转发(caddy-l4 插件)
- 审计日志、RBAC、开放 API Token(供 CI / AI Agent 调用)

## 7. 关键风险与对策

| # | 风险 | 对策 |
| --- | --- | --- |
| 1 | systemd 默认 `--config` 启动,重启丢失 API 改动 | Caddyfile 落盘;面板启动时校验 DB 与磁盘配置一致性并告警 |
| 2 | Admin API 无鉴权 | 强制 unix socket/localhost;面板做唯一入口与鉴权 |
| 3 | Caddyfile 反向导入无法 100% 还原模型 | MVP 以面板为事实源;导入降级为"保留自定义片段" |
| 4 | 多端并发写冲突 | `Etag`/`If-Match` 乐观锁 + 面板内单写者队列串行下发 |
| 5 | 限流/DNS challenge 等依赖 xcaddy 插件版二进制 | 功能标注插件依赖;提供带插件的 Docker 镜像变体 |
| 6 | autosave.json 膨胀导致 API 卡顿(社区已报 issue) | 不依赖 autosave;面板 DB 为事实源,Caddyfile 为持久层 |

## 8. 参考资料

- [Caddy Admin API 官方文档](https://caddyserver.com/docs/api)
- [Caddy API Tutorial](https://caddyserver.com/docs/api-tutorial)
- [Keep Caddy Running(持久化/resume)](https://caddyserver.com/docs/running)
- [Monitoring Caddy with metrics](https://caddyserver.com/docs/metrics)
- [社区讨论:CaddyManager 0.0.1 发布帖](https://caddy.community/t/caddymanager-0-0-1-a-web-ui-to-manage-multiple-caddy-instances/31396)
- [Nginx UI(架构参考)](https://github.com/0xJacky/nginx-ui)
- [gsmlg-dev/caddy-admin-ui](https://github.com/gsmlg-dev/caddy-admin-ui)
- [caddymanager/caddymanager](https://github.com/caddymanager/caddymanager)

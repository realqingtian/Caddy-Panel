# 技术栈选型明细

> 状态:已实现并随项目演进(M1–M3 交付,2026-10-11);实际版本以后端 go.mod / 前端 package.json 为准。
> 决策来源:[实现方案调研](./research-implementation-plan.md);版本为本机已验证或当前稳定版。

## 1. 总览

| 层 | 选型 | 版本基准 |
| --- | --- | --- |
| 后端语言/运行时 | Go | 1.27(本机 1.27.1) |
| 后端 HTTP 框架 | **Gin** | v1.12(Go 生态 star 最高、使用最广的 Web 框架,事实标准) |
| 参数校验 | go-playground/validator v10(Gin binding 内置同一引擎) | v10 |
| ORM / 数据库 | GORM + SQLite(glebarez 纯 Go 驱动) | GORM v2 |
| 鉴权 | JWT(golang-jwt/jwt v5)+ bcrypt(x/crypto) | — |
| 配置管理 | spf13/viper(env + yaml,主流标配) | v1 |
| 日志 | slog(Go 1.21+ 标准库,当前官方推荐) | stdlib |
| CORS | gin-contrib/cors(Gin 官方贡献库) | latest |
| 后端错误消息 i18n | nicksnyder/go-i18n(按 `Accept-Language` 返回中/英,文案集中语言包;默认 zh-CN) | v2 |
| API 文档 | swaggo/gin-swagger(注解生成 OpenAPI,Swagger UI 托管在 `/swagger`,前后端契约同源) | **M1 引入** |
| 前端框架 | React + TypeScript | React 19、TS 6(typescript-eslint 当前兼容上限) |
| 前端构建 | Vite | 8.x(2026-10 脚手架初始化时最新稳定版) |
| 组件库(UI 框架) | **Ant Design** | **v6**(2025 正式发布;原生支持 React 19、语义化 DOM,无需 v5 时代的 patch 包) |
| **包管理器** | **Bun**(本机 1.4.2) | `bun install` / `bun add` / `bun run`,锁文件 `bun.lock`;仅作依赖与脚本管理,构建仍走 Vite |
| 前端路由 | React Router | v7(库模式) |
| 服务端状态 | TanStack Query | v5 |
| 本地状态 | Zustand | v5 |
| HTTP 客户端 | axios | v1 |
| 前端工程化 | ESLint 10(flat config)+ Prettier;测试 Vitest | 当前主流(2026-10 初始化版本) |
| 国际化 | react-i18next(i18next),M1 即内置中/英 | React 生态事实标准;antd locale 经 ConfigProvider 同步切换 |
| 主题 | antd 6 theme algorithm(亮/暗/跟随系统),偏好持久化 | 组件库原生能力,零额外依赖 |
| 配置编辑器 | @monaco-editor/react | VS Code 同源内核,自定义 Caddyfile 语法高亮 |
| 部署 | Docker + docker compose;单二进制(go:embed 前端产物) | — |

## 2. 关键选型理由

**为什么 Bun 只做包管理、构建仍用 Vite?**
Bun 的 install/脚本执行比 npm 快且本机已就绪;但 Vite 插件生态(React Fast Refresh、Monaco worker 处理)最成熟,风险最低。后续如需可平滑切 `bun build`。

**为什么 React 19 + Vite 7?**
当前稳定大版本组合,生态(TanStack Query v5、React Router v7、zustand v5、antd 6)全部就绪;Vite 仍是 React 社区构建工具的主流默认选,dev 启动快、插件生态全。

**为什么 SQLite + glebarez 驱动?**
单文件零部署;glebarez/sqlite 是纯 Go 实现(无 CGO),`go test`、交叉编译、scratch/alpine 镜像都不受影响。

**为什么 Gin?**
Go 生态事实标准的 Web 框架(star/下载量最高,资料与中间件生态最全):参数绑定即 `ShouldBindJSON`,内置 validator 校验,中间件(JWT/CORS/审计/Recover)全是社区标准件。面板的性能瓶颈在 Caddy Admin API 调用而非框架,Gin 的性能绰绰有余 —— 选它就是选"最主流 + 最省心"。

**为什么 antd 6?**
中后台 UI 框架的国内事实标准;v6 是当前大版本,原生支持 React 19(官方移除了 v5 时代的兼容代码,不再需要 `@ant-design/v5-patch-for-react-19`),组件语义化 DOM 结构,开箱即用覆盖本项目的表单/表格/抽屉/差异对比全部场景。

**为什么 Caddyfile 校验优先离线?**
import `github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile` 可在 Panel 进程内完成 Caddyfile→JSON 适配,校验不依赖 Caddy 存活;Caddy 未运行时仍可"保存但不下发"。代价是引入较重的 caddy 依赖(可接受,同生态)。在线兜底:`POST /adapt`。

## 3. 依赖清单

### 前端(Bun)

```bash
bun add react react-dom antd@6 react-router-dom @tanstack/react-query zustand axios dayjs \
  i18next react-i18next
bun add -d typescript vite @vitejs/plugin-react @types/react @types/react-dom \
  @monaco-editor/react monaco-editor eslint@9 prettier vitest
```

### 后端(Go)

```bash
go get github.com/gin-gonic/gin github.com/gin-contrib/cors
go get gorm.io/gorm github.com/glebarez/sqlite
go get github.com/golang-jwt/jwt/v5 golang.org/x/crypto
go get github.com/spf13/viper github.com/go-playground/validator/v10
go get github.com/nicksnyder/go-i18n/v2  # 后端错误消息中/英(FR-603),T2 起使用
go get github.com/caddyserver/caddy/v2   # 离线 adapt;体积大,M1 第 4 步才引入

# API 文档(handler 写 swaggo 注解 → swag init 生成,UI 挂 /swagger)
go get github.com/swaggo/swag/cmd/swag github.com/swaggo/gin-swagger github.com/swaggo/files
```

## 3.1 AI 协作文档资源(antd for-agents,M1 脚手架时接入)

官方为 AI 代理提供的资源(**勿依赖训练数据中的 antd v3–v5 知识,v6 差异大**),接入方式与配置片段统一维护在项目根 [AGENTS.md](../AGENTS.md):

| 资源 | 用途 |
| --- | --- |
| `https://ant.design/llms.txt` / `llms-full-cn.txt` | LLM 友好文档索引/全量(按需取,勿整包塞上下文) |
| `https://ant.design/components/<name>.md` | 单组件 markdown 直链,查 API 首选,不爆 context |
| `npx -y @ant-design/cli mcp` | MCP Server:`antd info/demo/token/changelog/lint/doctor`,跨版本 API 对比、废弃 API 检查 |
| `npx skills add ant-design/ant-design-cli` | 安装官方 SKILL.md,自动提醒正确用法 |
| `https://ant.design/docs/react/for-agents-cn` | 代理使用指南与开箱即用提示词 |

## 4. 开发与运行形态

| 形态 | 命令(约定) | 说明 |
| --- | --- | --- |
| 本地开发 | `go run ./cmd/panel`(后端 :8080)+ `bun run dev`(前端 :5173,proxy /api) | 依赖本机 Caddy(:2019);无 Caddy 时仅配置管理不可下发 |
| Docker | `docker compose up`(deploy/ 下:panel + caddy 两容器) | 面板经容器网络访问 caddy:2019,生产建议面板 sidecar 共享 netns + unix socket |
| 单二进制 | `go:embed` 前端 `dist/`,一个可执行文件承载 UI + API | 交付主形态,对标 Nginx UI |

## 5. 工程规范(约定)

- 前端:函数组件 + hooks;页面级目录自包含(pages/Sites/ 下放私有组件);API 层按域拆分文件,类型集中 `api/types.ts`;提交前 `bun run lint`(ESLint + Prettier)。
- 后端:标准 Go 布局(cmd/internal),三层结构 **handler(协议转换)→ service(业务逻辑)→ store / caddy(数据与基础设施)**;handler 不写业务,业务全部落在 service;日志用 `slog`;错误统一 `{code,message,detail}`。
- 两者接口契约:REST + JSON,见 [M1 设计第 5 节](./m1-design.md)。

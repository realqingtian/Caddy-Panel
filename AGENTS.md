# AGENTS.md — AI 代理与协作者须知

Caddy Panel:可视化管理 Caddy 的面板(Go + Gin 后端 / React 19 + Ant Design 6 前端 / Bun 包管理)。

**必读文档**(按顺序):[进度管控](docs/progress.md) · [需求规格 PRD](docs/requirements.md) · [技术栈明细](docs/tech-stack.md) · [项目架构与目录设计](docs/project-structure.md) · [M1 详细设计](docs/m1-design.md) · [实现方案调研](docs/research-implementation-plan.md)

**会话流程**:开发会话开始先读 `docs/progress.md` 接续上下文,按看板下一任务开工;会话结束前更新进度看板与决策日志。

## 铁律

1. **antd 用 v6,不要信训练数据里的 v3–v5 API** —— 官方明确警告 v6 的组件 API/约定/文件结构与旧版不同。写组件前先查文档(见下)。
2. handler 只做协议转换,**业务全部写进 service 层**;审计由 `middleware/audit.go` 唯一写入,service 不得重复写 `audit_logs`。
3. 一切配置变更必须走 `caddy/pipeline`(渲染 → 校验 → load → 原子落盘 → 快照),禁止绕过管道直连 Admin API 写配置。
4. 错误响应统一 `{code, message, detail}`;接口必须写 swaggo 注解。
5. 前端依赖只用 Bun(`bun add` / `bun run`),不引入 npm/pnpm/yarn。

## 查 antd 6 文档的正确姿势

| 场景 | 用法 |
|---|---|
| 查单个组件 API(首选) | `https://ant.design/components/<name>.md`(如 `table.md`、`form.md`) |
| 找有哪些文档 | `https://ant.design/llms.txt`(中文全量:`llms-full-cn.txt`,按需分段取,勿整包塞上下文) |
| 跨版本 API 对比 / 废弃检查 | `npx -y @ant-design/cli` 的 `antd changelog` / `antd lint` / `antd doctor` |
| 需要设计语言/主题 Token 上下文 | `https://ant.design/design.md` |

**MCP Server 配置**(脚手架阶段接入客户端):

```json
{
  "mcpServers": {
    "antd": {
      "command": "npx",
      "args": ["-y", "@ant-design/cli", "mcp"]
    }
  }
}
```

**SKILL 安装**(可选):`npx skills add ant-design/ant-design-cli`

官方代理指南:<https://ant.design/docs/react/for-agents-cn>

## swaggo 注解约定

每个 handler 上方写注解(`@Summary`/`@Tags`/`@Param`/`@Success`/`@Failure`/`@Router`),改接口必同步改注解;生成命令 `swag init -g cmd/panel/main.go -o docs_swag`;开发态 UI:`/swagger/index.html`。

## 常用命令

```bash
# 后端(backend/)
go run ./cmd/panel                    # 启动 :8080
go test ./...                         # 单测(集成测试带 -tags integration)

# 前端(frontend/)
bun install                           # 安装依赖
bun run dev                           # Vite :5173,/api 代理到 :8080
bun run lint && bun run build         # 提交前必过

# 一键生成 API 文档
cd backend && swag init -g cmd/panel/main.go
```

## 提交前检查

- [ ] `bun run lint` 通过;`go vet ./...` 通过
- [ ] 新接口有 swaggo 注解且 Swagger 可见
- [ ] 涉及配置变更的代码走 pipeline 且失败分支有测试
- [ ] UI 组件用法与 antd 6 文档核对过(非记忆中的 v5)

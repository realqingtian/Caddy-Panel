# AGENTS.md — AI 代理与协作者须知

Caddy Panel:可视化管理 Caddy 的面板(Go + Gin 后端 / React 19 + Ant Design 6 前端 / Bun 包管理)。

**必读文档**(按顺序):[进度管控](docs/progress.md) · [需求规格 PRD](docs/requirements.md) · [技术栈明细](docs/tech-stack.md) · [项目架构与目录设计](docs/project-structure.md) · [M1 详细设计](docs/m1-design.md) · [M2 详细设计](docs/m2-design.md) · [M3 详细设计](docs/m3-design.md) · [实现方案调研](docs/research-implementation-plan.md)

**会话流程**:开发会话开始先读 `docs/progress.md` 接续上下文,按看板下一任务开工;会话结束前更新进度看板与决策日志。

## 铁律

1. **antd 用 v6,不要信训练数据里的 v3–v5 API** —— 官方明确警告 v6 的组件 API/约定/文件结构与旧版不同。写组件前先查文档(见下)。
2. handler 只做协议转换,**业务全部写进 service 层**;审计由 `middleware/audit.go` 唯一写入,service 不得重复写 `audit_logs`。
3. 一切配置变更必须走 `caddy/pipeline`(渲染 → 校验 → load → 原子落盘 → 快照),禁止绕过管道直连 Admin API 写配置。
4. 错误响应统一 `{code, message, detail}`;接口必须写 swaggo 注解。
5. 前端依赖只用 Bun(`bun add` / `bun run`),不引入 npm/pnpm/yarn。
6. **UI 文案一律走 i18n**:写入 `src/locales/zh-CN.json` 与 `en-US.json` 两份语言文件,禁止硬编码任何语言文案。

## 开发规范(代码质量要求,与铁律同级约束)

> 核心原则:**一个文件/模块只做一件事**。任何"顺手在这里再处理一下"都是坏味道 —— 单一职责优先于省事。

### 单一职责与模块化

- 各层边界不可越界:handler 不写业务、service 不直接拼 SQL/不发 HTTP、store 不关心业务含义、caddy 包不读业务表;改一个需求原则上只动一层;
- 单文件建议 ≤ 300 行、单函数 ≤ 50 行、函数嵌套 ≤ 3 层(超了先拆函数/拆文件,用 early return 压嵌套);
- 新增能力先问"放哪一层":表单校验前端做一层、service 用 validator 再做一层(**永远不信任前端入参**);
- 前端:组件只管展示与交互;数据获取一律走 `api/` 层 + TanStack Query,业务规则不写进组件;组件复用两次就提为 `components/`,逻辑复用两次就提为 `hooks/`/`utils/`;
- 后端:同一逻辑出现第三次必须抽公共函数;但**不过度抽象** —— 没有第二次出现不预设"将来可能通用"。

### 可读性

- 命名见名知义:禁止 `a`/`tmp`/`data1`/`handle2`;Go 导出符号必须有 doc comment;布尔命名读起来像判断(`enabled`、`httpsRedirect`,不要 `flag1`);
- 禁止魔法值:域名、路径、阈值、指令名等一律常量/枚举,并集中在所属包顶部或 `consts`;
- 注释只写**为什么**(约束、坑、取舍),不复述代码做了什么;复杂取舍(如单写者锁、全量渲染)必须留注释指向对应设计文档章节;
- 用户可见的错误信息必须是人话并带原因,后端错误原样透传 Caddy 错误原文,禁止吞成 "操作失败"。

### 健壮性

- 所有错误必须显式处理:禁止 `_ = err`;向上传递用 `%w` wrap 并带上 下文(`fmt.Errorf("渲染站点 %s: %w", site.Name, err)`);
- 一切外部调用(Caddy Admin API、文件系统、SQLite)必须设超时、写失败分支、有对应测试;HTTP client 不允许无 timeout;
- 后台任务/goroutine 必须可退出(context 贯穿),启动项随 graceful shutdown 收敛;
- 边界输入必防:空列表、超长域名、超大日志 tail(强制上限)、非法 JSON;文件写入一律临时文件 + rename 原子写;
- 共享状态并发访问必须加锁(下发全局单写者锁已是范例);新增并发逻辑须在 PR 描述里说明锁策略。

### 可维护性

- 依赖一律接口注入(service 定义接口、handler 持接口),测试用 mock/fake,禁止在函数内部直接构造依赖;
- 配置不硬编码:可调参数进 env 或 settings 表(参考已有 `PANEL_*` 约定);
- 改了行为必须同步改:swaggo 注解、两份语言文件、相关设计文档;文档与代码不一致视为 bug;
- 每个失败分支至少一条测试(service 层接口 mock 覆盖),不测"只有成功路径"的功能。

## 查 antd 6 文档的正确姿势

| 场景 | 用法 |
| --- | --- |
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
- [ ] 新增 UI 文案已同时补入 zh-CN.json 与 en-US.json

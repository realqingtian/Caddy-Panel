# 开发进度管控

> 本文档是**进度与决策的唯一维护处**。每次开发会话开始先读本文接续上下文,结束前更新状态。
> 任务定义见 [M1 设计第 8 节](./m1-design.md);需求验收标准见 [PRD](./requirements.md)。状态图例:⬜ 未开始 / 🔵 进行中 / ✅ 完成 / ⛔ 阻塞。

## 1. 当前状态

**阶段:设计完成,待启动编码(M1-T1 脚手架)。**
下次会话建议动作:执行 T1(前后端脚手架 + Bun 初始化 + antd MCP/Skill 接入)。

## 2. 里程碑

| 里程碑 | 判定标准 | 状态 |
|---|---|---|
| M0 设计与规划 | 文档体系定稿(调研/技术栈/架构/M1 设计/PRD/本文件) | ✅ |
| M1 MVP 可用 | T1–T9 全部完成,PRD 中 ✅ 需求验收全过,Docker Compose 一键可跑 | ⬜ |
| M2 进阶 | 见 PRD 第 5 节占位,启动前再细化 | ⬜ |
| M3 平台化 | 同上 | ⬜ |

## 3. 任务看板

### M0 — 设计与规划

| ID | 任务 | 状态 | 完成日期 | 产出 |
|---|---|---|---|---|
| T0.1 | 实现方案调研 | ✅ | 2026-10-10 | docs/research-implementation-plan.md |
| T0.2 | 技术栈定稿 + 架构与目录设计 | ✅ | 2026-10-10 | docs/tech-stack.md · docs/project-structure.md |
| T0.3 | 架构评审与修复(service 分层、并发一致性、测试约定等) | ✅ | 2026-10-10 | 上述文档修订 |
| T0.4 | PRD + 进度管控文档 | ✅ | 2026-10-10 | docs/requirements.md · docs/progress.md |

### M1 — MVP

| ID | 任务 | 需求覆盖 | 状态 | 完成日期 | 备注 |
|---|---|---|---|---|---|
| T1 | 脚手架:前后端骨架可运行、compose 可起;接入 antd MCP/Skill(AGENTS.md) | FR-603/604 | ⬜ | | Bun 初始化前端;Gin 骨架;/healthz;i18n(中/英)与主题(亮/暗/跟随系统)基建 |
| T2 | 登录/JWT + SQLite 迁移 + 审计/限速中间件 | FR-10x | ⬜ | | |
| T3 | Caddy Admin client(/adapt /load /config /status) | FR-201 | ⬜ | | unix socket 支持 |
| T4 | Caddyfile 渲染引擎 + 校验 + pipeline(单写者/原子落盘) | FR-304/405 | ⬜ | | 渲染单测 golden file |
| T5 | 站点 CRUD API + 前端 Sites 页 | FR-30x | ⬜ | | |
| T6 | 配置中心(版本/diff/回滚/直编/系统设置)+ ConfigEditor/Settings 页 | FR-40x、FR-601 | ⬜ | | Monaco + Caddyfile 高亮 |
| T7 | 日志 tail API + Logs 页 | FR-50x | ⬜ | | 轮询 |
| T8 | 启动漂移检测 + Dashboard | FR-202 | ⬜ | | drift.go |
| T9 | Docker 镜像与 compose 打磨 + README | NFR-104 | ⬜ | | 多阶段构建 |

## 4. 决策与变更日志

| 日期 | 决策 | 理由摘要 |
|---|---|---|
| 2026-10-10 | 架构选方案 C:领域模型(DB 事实源)→ 渲染 Caddyfile → adapt 校验 → /load 热加载 | 兼顾热加载、重启恢复与可回滚;见调研第 4 节 |
| 2026-10-10 | 前端 React 技术栈(用户指定) | — |
| 2026-10-10 | 前端依赖管理用 Bun(用户指定) | 本机 1.4.2;仅做包管理,构建仍走 Vite |
| 2026-10-10 | 前后端均采用当前主流新版:Go 1.27 + Gin v1.12 / React 19 + Vite 7 + antd 6(用户要求主流新技术) | antd 6 原生支持 React 19,无需 v5 patch 包 |
| 2026-10-10 | 后端补 service 层(handler→service→store/caddy 三层) | 评审意见:业务逻辑需独立可测层 |
| 2026-10-10 | 评审修复:下发全局单写者锁、失败回滚语义、审计唯一写入方、原子落盘、优雅停机、首启随机密码、/healthz、登录限速、drift 独立模块、测试约定 | 见 progress 同日评审记录;细节在 project-structure.md 第 3.1、5 节 |
| 2026-10-10 | swaggo/OpenAPI 转正为 M1 范围(用户拍板) | 注解成本低,前后端契约同源 |
| 2026-10-10 | 引入 antd 官方 for-agents 资源,新增 AGENTS.md(用户提议) | v6 与训练数据差异大,查文档防用废弃 API |
| 2026-10-10 | License 选 Apache-2.0(用户拍板) | 宽松且含专利授权条款,利于社区采用 |
| 2026-10-10 | i18n(中/英)M1 即落地(用户拍板) | 免留历史包袱;react-i18next + antd ConfigProvider 同步 |
| 2026-10-10 | 暗色模式 M1 即落地(用户拍板) | antd 6 theme algorithm 原生支持,成本低 |
| 2026-10-10 | M2 范围细化完成(FR-701~706) | 用户拍板;详见 PRD 第 5 节 |

## 5. 待决事项

| 事项 | 状态 | 说明 |
|---|---|---|
| M2 范围细化 | ✅ | 已细化为 FR-701~706,见 PRD 第 5 节;M1 验收后复核优先级 |
| 是否上线 CI(GitHub Actions) | ⬜ | 建议 T9 完成后决定;最少做 lint + go test + build |

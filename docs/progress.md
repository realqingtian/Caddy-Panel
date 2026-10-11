# 开发进度管控

> 本文档是**进度与决策的活跃管理处**,服务于新需求、迭代、优化与 bug 修复:开工先读本文接上下文,结束前更新状态。
> 定位(2026-10-11 起):已完成里程碑、历史决策一律归档至 [docs/archive/completed-milestones.md](./archive/completed-milestones.md),本文只留**活跃看板与当前决策**。
> **归档铁律:以完整需求单元整体迁移** —— 一个里程碑/迭代/需求,其任务表、相关决策、验收记录必须同进同出,禁止只搬一部分(拆散会导致记录断裂、无法追溯);单元内最后一条任务完成即整个单元归档。
> 历史迭代明细见 [archive/iterations.md](./archive/iterations.md);状态图例:⬜ 未开始 / 🔵 进行中 / ✅ 完成 / ⛔ 阻塞。

## 1. 当前状态

**阶段:🎯 M4(用户系统)设计定稿 —— T4.1–T4.6 待开工(2026-10-11 设计评审完成)。**
M0–M3 已交付(任务表与决策已归档);迭代 I1–I3(UI 走查/布局壳/体验打磨)已完成 ✅(明细见 iterations.md)。
M4 设计见 [m4-users-design.md](./m4-users-design.md):本地多用户 + 权限点矩阵 + 可吊销会话 + TOTP 2FA。

## 2. 里程碑总览

| 里程碑 | 状态 | 归档记录 |
| --- | --- | --- |
| M0 设计与规划 / M1 MVP / M2 进阶 / M3 平台化 | ✅ | [completed-milestones.md](./archive/completed-milestones.md) |
| M4 用户系统 | 🔵 设计定稿 | 任务表见下节 |

## 3. 活跃任务看板

### M4 — 用户系统(设计见 [m4-users-design.md](./m4-users-design.md))

| ID | 任务 | 状态 | 完成日期 | 备注 |
| --- | --- | --- | --- | --- |
| T4.1 | sessions 表 + JWT sid 双检 + 吊销 API + 会话清扫 | ⬜ | | 可吊销会话基座 |
| T4.2 | 权限点模型(roleToPermissions + 中间件改造 + Token scope 对齐)+ 三角色迁移 | ⬜ | | editor 新增 |
| T4.3 | 用户管理 CRUD API(保护规则 + 审计)+ 前端用户管理页 | ⬜ | | |
| T4.4 | 自助链路:改密 + 会话列表/下线 + Header 个人设置 | ⬜ | | |
| T4.5 | TOTP 2FA 全链路(绑定/登录两段/恢复码/解绑) | ⬜ | | pquerna/otp |
| T4.6 | 真实验收(compose 全链路:权限矩阵逐点/踢会话/2FA/Token 兼容回归) | ⬜ | | 用户要求里程碑真实验收 |

### 迭代看板(UI 走查 / 优化 / bug 修复)

> 已完成迭代(I1 UI 走查+登录页、I2 布局壳、I3 体验打磨):状态见 [归档 §3](./archive/completed-milestones.md),明细见 [archive/iterations.md](./archive/iterations.md)。
> **制度**:进行中的迭代明细直接写在本节下方;迭代完成时,明细整段迁入归档卷(整单元迁移)。

| 迭代 | 任务 | 状态 | 备注 |
| --- | --- | --- | --- |
| — | 暂无进行中迭代 | — | 下一看板任务为 M4 T4.1 |

## 4. 决策日志(活跃期)

> 历史决策(M0–M3 及迭代期)已归档至 [completed-milestones.md §4](./archive/completed-milestones.md)。

| 日期 | 决策 | 理由摘要 |
| --- | --- | --- |
| 2026-10-11 | **M4 用户系统四项关键决策拍板**:①路线 A 自建本地用户系统(否决独立 IdP/OIDC 外接,保持一键部署);②权限粒度走**权限点矩阵 + 三级内置角色**(admin=*,editor 中间层,viewer 只读;不做自定义角色,users:manage 与 terminal:use 为 admin 硬规则);③JWT 升级为**有状态可吊销会话**(sessions 表 + sid claim 双检,改密/禁用即刻踢人);④本期纳入 **TOTP 2FA**(pquerna/otp,自助绑定 + 恢复码,解绑需密码+验证码) | 用户确认(A/细粒度/有状态/本期 2FA);M3 的 admin/viewer 硬编码判断全面改造为权限点;设计固化进 m4-users-design.md |
| 2026-10-11 | 进度文档重构:已完成里程碑/历史决策归档至 docs/archive/completed-milestones.md,progress.md 瘦身为活跃看板 + 当前决策(新需求/迭代/优化/bug 修复管理);**归档铁律:以完整需求单元(里程碑/迭代/需求)整体迁移,任务表+相关决策+验收记录同进同出,禁止拆散** | 用户提出并强调;降低信噪比,活跃文档聚焦"接下来做什么";防拆散保证历史记录可整体追溯 |
| 2026-10-11 | 迭代明细文档并入归档体系:iterations.md 整体迁入 docs/archive/(I1–I3 均已完成,本身即历史记录);新制度:进行中迭代的明细写在 progress.md 迭代看板下方,完成时整段迁入 archive/iterations.md —— 与"整单元迁移"铁律一致,活跃文档区收敛为 progress/requirements/tech-stack/project-structure/m4-users-design 五份 | 用户提出"iterations.md 是否可省";文档只保留"活跃"与"归档"两层,不再有中间层 |
| 2026-10-11 | **工作流文档全面审查**(通读活跃五份 + 核对实际代码/目录):①PRD 补 M4 需求正式化 FR-901~905(此前"需求唯一维护处"与 M4 开工之间断链,沿 M3 先例);②project-structure 目录树由 M1 规划视角校准为实际结构(补 agent/、全部 handler/service/model/页面、frontend scripts),env 表补 PANEL_MODE/CERT_STORAGE_PATH/PANEL_PUBLIC_URL 三项,修重复编号;③requirements 两处指向 progress 的失效链接改指归档(任务表已迁出);④tech-stack 修内部矛盾(Vite 7/8、ESLint 9/10),依赖清单标注为 M1 初始化清单 | 用户要求"全面审查是否最优有效";结论:两层(活跃/归档)结构成立,问题集中在"文档随实现演进"的滞后,已当轮修复 |

## 5. 待决事项

| 事项 | 状态 | 说明 |
| --- | --- | --- |
| 暂无 | — | — |

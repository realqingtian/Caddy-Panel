# 开发进度管控

> 本文档是**进度与决策的唯一维护处**。每次开发会话开始先读本文接续上下文,结束前更新状态。
> 任务定义见 [M1 设计第 8 节](./m1-design.md);需求验收标准见 [PRD](./requirements.md)。状态图例:⬜ 未开始 / 🔵 进行中 / ✅ 完成 / ⛔ 阻塞。

## 1. 当前状态

**阶段:M1 编码进行中 —— T5 站点 CRUD 已完成,下一任务 T6(配置中心:版本/diff/回滚/直编/系统设置 + ConfigEditor/Settings 页,Monaco + Caddyfile 高亮)。**
T6 注意事项:store 已有 CreateConfigVersion;前端首次引入 Monaco(@monaco-editor/react);回滚复用 service 层"下发→落库"编排;FR-601 设置页含登录锁定参数(立即生效)与 metrics/global_extra 等。

## 2. 里程碑

| 里程碑 | 判定标准 | 状态 |
| --- | --- | --- |
| M0 设计与规划 | 文档体系定稿(调研/技术栈/架构/M1 设计/PRD/本文件) | ✅ |
| M1 MVP 可用 | T1–T9 全部完成,PRD 中 ✅ 需求验收全过,Docker Compose 一键可跑 | ⬜ |
| M2 进阶 | 见 PRD 第 5 节占位,启动前再细化 | ⬜ |
| M3 平台化 | 同上 | ⬜ |

## 3. 任务看板

### M0 — 设计与规划

| ID | 任务 | 状态 | 完成日期 | 产出 |
| --- | --- | --- | --- | --- |
| T0.1 | 实现方案调研 | ✅ | 2026-10-10 | docs/research-implementation-plan.md |
| T0.2 | 技术栈定稿 + 架构与目录设计 | ✅ | 2026-10-10 | docs/tech-stack.md · docs/project-structure.md |
| T0.3 | 架构评审与修复(service 分层、并发一致性、测试约定等) | ✅ | 2026-10-10 | 上述文档修订 |
| T0.4 | PRD + 进度管控文档 | ✅ | 2026-10-10 | docs/requirements.md · docs/progress.md |

### M1 — MVP

| ID | 任务 | 需求覆盖 | 状态 | 完成日期 | 备注 |
| --- | --- | --- | --- | --- | --- |
| T1 | 脚手架:前后端骨架可运行、compose 可起;接入 antd MCP/Skill(AGENTS.md) | FR-603/604 | ✅ | 2026-10-10 | 后端 /healthz+Swagger+SPA 托管+优雅停机;前端 i18n 中英、主题三态、AdminLayout+占位页;compose 双容器验证通过。版本:antd 6.6.5 / Vite 8 / TS 6 / ESLint 10 |
| T2 | 登录/JWT + SQLite 迁移 + 审计/限速中间件 | FR-10x | ✅ | 2026-10-10 | GORM 5 表 AutoMigrate;首启 seed(PANEL_ADMIN_PASSWORD/随机);JWT+bcrypt;LoginGuard 来源锁定(settings 运行时可调);审计唯一写入方;后端错误消息 i18n(Accept-Language);前端接真实登录 |
| T3 | Caddy Admin client(/adapt /load /config /status) | FR-201 | ✅ | 2026-10-10 | http(s):// 与 unix:// 双协议;请求超时 10s、响应上限 32MiB;If-Match 并发防护;Caddy 错误原文以 HTTPError 透传;Ping 读 /config 判活 |
| T4 | Caddyfile 渲染引擎 + 校验 + pipeline(单写者/原子落盘) | FR-304/405 | ✅ | 2026-10-10 | 模板嵌入 templates 包(转换层厚/模板薄);离线适配 modules/standard 全注册;golden 渲染测试 + "产物必过离线校验"契约测试;pipeline 单写者锁 + tmp+rename 原子写 + Deploy 开关(Caddy 未运行可保存);新增 PANEL_CADDY_ADMIN_BIND;真机 /load 集成验证随 T5 联调 |
| T5 | 站点 CRUD API + 前端 Sites 页 | FR-30x | ✅ | 2026-10-10 | service 编排(先 Caddy 后 DB)+ 域名查重 + dry_run 预检 + 版本快照(成败都留痕);compose 真 Caddy 联调:创建→反代生效→停用下线→启用恢复→删除,域名冲突 409/非法输入 400/下发失败 422(含 Caddy 原文);前端 Sites 页 Table+动态 Drawer+启停+删除 |
| T6 | 配置中心(版本/diff/回滚/直编/系统设置)+ ConfigEditor/Settings 页 | FR-40x、FR-601 | ⬜ | | Monaco + Caddyfile 高亮 |
| T7 | 日志 tail API + Logs 页 | FR-50x | ⬜ | | 轮询 |
| T8 | 启动漂移检测 + Dashboard | FR-202 | ⬜ | | drift.go |
| T9 | Docker 镜像与 compose 打磨 + README | NFR-104 | ⬜ | | 多阶段构建 |

## 4. 决策与变更日志

| 日期 | 决策 | 理由摘要 |
| --- | --- | --- |
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
| 2026-10-10 | 登录锁定参数支持 .env 默认 + 设置页运行时调整;JWT 有效期仅 .env 配置、不做运行时修改(用户要求/拍板) | 爆破防护需免重启调整;JWT TTL 变更影响面在全部会话,改 env 重启即可 |
| 2026-10-10 | 二轮评审(演进视角):预留 raw 片段逃生舱(sites.raw_extra / settings.global_extra)、补 FR-308 证书状态查看、FR-601 加 metrics 开关、明确单实例约束与 Caddy 版本对齐、写明 M2/M3 数据迁移路径 | 全部为低成本预留,扫清 M2/M3 迭代障碍 |
| 2026-10-10 | AGENTS.md 新增"开发规范"章节:单一职责(文件 ≤300 行/函数 ≤50 行/层间不越界)、可读性、健壮性(错误处理/超时/原子写)、可维护性(接口注入/失败分支必测)(用户要求) | 防止"一个文件做完所有事"的上帝文件,质量约束前置到编码前 |
| 2026-10-10 | 后端错误消息 i18n 纳入 M1(FR-603 扩展):按请求 `Accept-Language` 返回中/英 `message`(默认 zh-CN),`detail` 原文不翻译;选型 go-i18n,前端 axios 统一携带语言头 | 用户拍板;前端已切语言,后端报错仍是中文会割裂体验 |
| 2026-10-10 | 前端 lint 维持 ESLint 10 插件生态,Oxlint 暂不引入;M1 完成后再评估(替代/混用/维持) | react-refresh、typescript-eslint 等插件能力是刚需;Oxlint 速度优势明显但生态对等性待比对(见待决事项) |
| 2026-10-10 | 登录失败锁定实现为 service.LoginGuard(进程内存),未按初版规划放 middleware/limiter.go —— 锁定是业务规则且参数每次从 settings 读取(运行时可调立即生效),service 层可注入 mock 单测;middleware 只留协议层(Language/JWTAuth/Audit) | 分层正确性优先于文档既有命名,progress 留痕 |
| 2026-10-10 | JWT 密钥首启生成后持久化到 settings 表,重启沿用库内密钥 | 避免随机密钥导致每次重启踢掉全部会话(FR-103 会话连续性) |
| 2026-10-10 | 站点编排改为"先 Caddy 后 DB":pipeline 成功后才提交站点变更,失败零 DB 变更,消除补偿回滚中间态;Caddy 生效而 DB 提交失败的崩溃窗口由启动漂移检测(T8)兜底。与设计第 3 节"落库→下发"顺序不同,以 3.1 一致性语义为准 | 补偿回滚本身可能失败会产生更难解释的中间态 |
| 2026-10-10 | 渲染全局块新增 PANEL_CADDY_ADMIN_BIND(默认 localhost:2019):/load 会应用全局块,若绑定与 Panel 实际可达地址不一致,首次下发后 Panel 即失联;compose 中为 caddy:2019 | T5 联调发现的真实故障模式 |

## 5. 待决事项

| 事项 | 状态 | 说明 |
| --- | --- | --- |
| M2 范围细化 | ✅ | 已细化为 FR-701~706,见 PRD 第 5 节;M1 验收后复核优先级 |
| 是否上线 CI(GitHub Actions) | ⬜ | 建议 T9 完成后决定;最少做 lint + go test + build |
| Oxlint 评估(替代/混用 ESLint) | ⬜ | **M1 MVP 完成后提醒用户评估**:Oxlint 速度优势明显,但 react-refresh/typescript-eslint 等插件能力需逐项比对;届时决定"替代 / 预检+深检混合 / 维持现状" |

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
| M4 用户系统 | 🔵 T4.1–T4.5 完成(S1 自测全绿),T4.6 真实验收待执行 | 任务表见下节 |

## 3. 活跃任务看板

### M4 — 用户系统(设计见 [m4-users-design.md](./m4-users-design.md))

| ID | 任务 | 状态 | 完成日期 | 备注 |
| --- | --- | --- | --- | --- |
| T4.1 | sessions 表 + JWT sid 双检 + 吊销 API(含 logout)+ 会话清扫 + 外部认证预留字段(auth_source/provider_subject/password_hash 可空,CredentialVerifier 抽象) | ✅ | 2026-10-11 | 部分唯一索引:username 存活唯一 / 外部锚点复合唯一 |
| T4.2 | 权限点模型(roleToPermissions + 中间件改造 + Token scope 对齐)+ 三角色迁移(editor 常量新增) | ✅ | 2026-10-11 | 历史空 role 视为 admin 行为保留 |
| T4.3 | 用户管理 CRUD API(软删/保护规则/must_change_password 及其服务端强制/审计)+ 前端用户管理页(最后活跃/会话强制下线) | ✅ | 2026-10-11 | 保护规则:自操作拒绝 / 最后一个可用 admin 保护 |
| T4.4 | 自助链路:个人资料(昵称/邮箱/头像上传)+ 改密(策略校验)+ 会话列表/下线/下线其他 + Header 个人设置 + 首登强制改密弹窗 + Gravatar 镜像设置 | ✅ | 2026-10-11 | 头像三级回退:上传 > Gravatar(镜像可配)> 首字母 |
| T4.5 | TOTP 2FA 全链路(绑定/登录两段/恢复码 + 重新生成/解绑) | ✅ | 2026-10-11 | pquerna/otp;密钥 AES-GCM 加密落库 |
| T4.6 | 真实验收:按 §10 验收流程执行(验收服务器部署 → V0–V28 用例 → 缺陷清零 → 记录归档) | ⬜ | | S1 全绿;部署环境就绪(ops/)|

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
| 2026-10-11 | **M4 设计评审**(用户要求复审):发现并修订 8 项 —— ①砍除无消费的 audit:read 权限点(审计查看移入非目标);②补存量会话迁移策略(无 sid 旧 JWT 一律 401,上线全员重登一次,不留兼容旁路);③删除用户改软删除(保审计 user_id 可追溯);④2FA 二段失败计入 LoginGuard;⑤补 admin 视角会话 API(GET/DELETE /users/:id/sessions);⑥新增 §3.4 角色→菜单可见性表(前端显隐与验收依据);⑦明确仪表盘跟随聚合来源 read、不单设权限点;⑧明确权限点由 claims.role 推导不查库,一致性靠角色变更必吊销 | 设计文档先于编码二次评审;全部修订已回写 m4-users-design.md,PRD FR-90x 验收标准同步 |
| 2026-10-11 | users 表补资料字段与头像方案(用户拍板):新增 `nickname`/`email`(仅资料字段,不做验证/找回/唯一约束);头像初版按 Gravatar 派生;随后用户明确**必须支持手动上传** → 定稿三级回退链(自定义上传 > Gravatar > 首字母),users 增 `avatar_path`(服务端生成不可猜文件名,存 data/avatars/,静态路由不挂鉴权靠不可猜性 + 内容嗅探白名单 2MiB 上限);新增自助接口 PATCH /auth/profile 与 avatar 上传/移除、sessions/others 一键下线;个人设置页功能清单成文(§8.1:资料/改密/2FA/登录设备四区块);新密码策略 ≥8 字符 | 用户要求"必须有 avatar 字段存用户上传的" + "梳理个人资料界面该有的功能";字段定义完整化见 m4-users-design §4.1/§8.1 |
| 2026-10-11 | Gravatar 镜像站可配置(用户拍板,防官方源不可达):settings 新键 `gravatar_mirror_url`(空=官方,设置页常规 Tab AutoComplete 预设 + 自定义,http(s) 校验);预设含官方/gravatar.qingtian.dev/cravatar.cn/weavatar.com/loli.net;前端经 GET /settings(全角色可读)获取后拼头像 URL;镜像须兼容 Gravatar URL 协议 | 用户要求支持镜像站并给出 qingtian.dev 预设;第三方镜像可用性不受控,故必须允许自定义;回退链兜底首字母保证无破图 |
| 2026-10-11 | **M4 需求范围二轮评审**(对照主流用户系统逐域盘点),补 8 项:①**退出登录改服务端吊销当前会话**(实锤:M1 起退出纯前端,旧 token 有效至过期,会话表后语义必须闭合);②**首登强制改密**(`must_change_password`,admin 创建/重置密码场景标配,前端不可关弹窗);③恢复码重新生成(旧码作废,需当前验证码);④新密码不得与旧密码相同;⑤username 创建后不可改为硬规则(删除重建,审计锚点稳定);⑥用户管理列表加"最后活跃"列;⑦明确 API Token 不受 2FA 约束(机器凭证不经登录链路);⑧"记住我"列入非目标 | 用户要求"审查 M4 全部需求功能";对照 GitHub/Portainer/Grafana 账号体系;①②为真实缺口,其余为语义明确化;PRD FR-90x 与设计文档已同步 |
| 2026-10-11 | M4 增外部认证扩展预留(用户要求,应对后续 LDAP/OIDC/GitHub/Google 接入):新增 FR-906;本期落地**字段层**(users 加 auth_source/provider_subject + 复合唯一索引、password_hash 可空化)与**链路层抽象**(CredentialVerifier 接口,local 唯一实现;OAuth 预留 /auth/oauth/{provider}/start\|callback 路由位,回调汇入既有会话/2FA/审计下游);账号供给/角色映射/认证源管理 UI 等决策留给接入迭代;外部认证列为 M5 候选 | 用户要求预留扩展;设计原则:预留以"未来零改表、增量插入"为准绳,不过度实现 |
| 2026-10-11 | **M4 设计三轮评审**(绕过者视角流程推演),修 2 漏洞 + 1 同步:**①must_change_password 服务端强制**——原设计仅前端弹窗,绕过前端直接调 API 不受限;改为鉴权中间件白名单放行(改密/profile/avatar/logout),其余 403 password_change_required;**②totp_recovery 长度错误**——512 装不下 10 个 bcrypt 哈希(约 609 字符),改 1024;③任务拆解表同步三轮评审新增功能到 T4.1–T4.5。**两项决策用户确认**:④Caddyfile 直编仅 admin(拆 `config:direct-edit` 高危权限点,editor 保留回滚)——直编可改全局块影响面大;⑤密码策略维持仅 ≥8 字符长度(不做字符组合要求) | 用户要求三审 + 拍板两项确认点;高危权限点从 2 个增至 3 个(users:manage/terminal:use/config:direct-edit);PRD FR-902 与设计文档已同步 |
| 2026-10-11 | **M4 验收流程成文**(用户指出设计只有验收标准、缺验收流程):§10 重构为流程 + 用例——10.1 环境与准备(compose/账号矩阵/oathtool 出码/前置数据)、10.2 五阶段(S1 开发自测 → S2 部署 → S3 用例执行 → S4 缺陷分级与阻塞清零 → S5 记录归档)、10.3 用例清单 V0–V28(按 FR 分组逐条对应设计章节,含横切:头像/审计/i18n/回归);沿用"真实验收不做全 mock"铁律,T4.6 改为引用 §10 | 用户审查发现缺口;验收标准回答"验什么",验收流程回答"怎么验、过了才算"——两者缺一不可 |
| 2026-10-11 | **M4 开工前全面检查**(功能与文档,设计假设 vs 实际代码逐项核对):①核对 auth service/JWT 中间件/model/前端登录类型,§2 现状基线补齐鉴权中间件细节(双轨 scope 二值、RequireJWTAdmin 模式、历史空 role 视为 admin、Token 轨 UserID=0、model 无 editor 常量)——均为 T4.2 改造对象;②确认 password_hash 可空化后现有 bcrypt 校验行为天然安全(空串比对失败 → 401);③FR-901~906 ↔ T4.1~T4.6 ↔ 验收用例 V0–V28 追溯矩阵全覆盖,唯一未归属功能(Gravatar 镜像设置)补入 T4.4;④markdownlint 全绿(.markdownlint.jsonc 固化规则)。**结论:GO,T4.1 可开工** | 用户要求开工前全面检查;发现均为文档补齐级,无设计缺陷;文档未提交 git(连同 I2/I3 代码待一并提交) |
| 2026-10-11 | **验收服务器纳管**(用户购入新服务器):Ubuntu 24.04/2C/2G/40G @ 189.24.79.203(域名 `caddy-panel.realqingtian.com` 已解析);项目根新增 `ops/` 本地目录(**整目录 .gitignore 不入库**):server.md(连接信息/概况/部署约定)、keys/(密钥副本 600)、DEPLOY-LOG.md(**制度:每次部署/维护操作必须登记留底**);部署形态定为 Caddy 边缘反代 + 自动 HTTPS(域名访问),panel 不裸露端口;M4 验收环境从本地 compose 升级为该服务器(§10.1) | 用户提供服务器与域名;密钥不入库双保险(ops/ + *.pem);服务器信息与操作记录属地管理,与仓库文档分离 |

## 5. 待决事项

| 事项 | 状态 | 说明 |
| --- | --- | --- |
| 暂无 | — | — |

# M4 详细设计 —— 用户系统(多用户 / 细粒度权限 / 会话 / 2FA)

> 状态:设计定稿(2026-10-11,四项关键决策经用户拍板,见 [progress.md](./progress.md) 决策日志)。
> 前置:[PRD](./requirements.md) · [M3 设计](./archive/m3-design.md)(RBAC admin/viewer、API Token 现状)。
> 本文是实现与验收的唯一依据;实现完成后按里程碑惯例归档至 `docs/archive/`。

## 1. 目标与非目标

### 1.1 目标

1. **本地多用户**:管理员创建/禁用/重置用户,告别单 admin;
2. **细粒度权限**:角色 → 权限点矩阵,路由校验从"两级 role"升级为权限点,内置三级角色;
3. **可吊销会话**:有状态 sessions 表,改密/禁用/降级即刻踢掉全部在线设备;提供登录设备列表与强制下线;
4. **TOTP 两步验证**:用户自助绑定(Google Authenticator 兼容),一次性恢复码;登录走密码 → 验证码两段;
5. **开放 API 兼容**:现有 `cpt_` Token 双轨认证不受破坏,scope 与新权限点对齐。

### 1.2 非目标(明确不做)

- 开放注册、邮箱验证、自助找回密码(自托管场景:管理员重置);email 仅为资料字段(Gravatar 头像来源),不承担凭证职能;
- 头像在线裁剪/缩放(上传仅做类型与大小校验,显示由前端 CSS 裁圆);admin 不代管他人头像;
- 自定义角色 / 资源级 ACL(内置角色 + 权限点矩阵已覆盖面板场景,自定义角色属过度设计;矩阵设计保证未来加角色只是加一行映射);
- OIDC/OAuth/LDAP 外接的**本期实现**(路线 C/B 被否;但字段与链路扩展点已随 M4 落地,见 §5.1——未来接入是增量插入,列为 M5 候选);
- passkey/WebAuthn;
- 审计日志查看接口与页面(审计写入既有能力不变,查看属独立迭代);
- 单用户会话并发数限制;
- "记住我"延长会话(会话时长统一 env `PANEL_JWT_TTL`,不做按设备差异化)。

## 2. 现状基线(M3 末)

| 项 | 现状 | 本期动作 |
| --- | --- | --- |
| users 表 | id/username/password_hash(bcrypt)/role/created_at,仅 seed 单 admin | 加列 + 管理接口 |
| 认证 | 无状态 JWT(签名密钥持久化 settings,有效期 env `PANEL_JWT_TTL`);claims 已含 uid/username/role | JWT 加 `sid` claim,与会话表双检 |
| RBAC | role 两级 admin/viewer(model 仅 RoleAdmin/RoleViewer 常量,无 editor),路由级 scope(read/write) | 升级为权限点矩阵(见 §3),新增 editor 常量 |
| 鉴权中间件 | 双轨(scope 二值):JWT 轨注入 uid/role/scope,scope 由 role 推导(admin→write,viewer→read,**历史空 role 视为 admin**);`RequireScope` 二值校验;`RequireJWTAdmin` 实现"Token 管理仅 JWT+admin" | 重构为权限点模型:`roleToPermissions(role)` 权限集 + `RequirePermission(点)` 校验;硬规则(users:manage/terminal:use/config:direct-edit 仅 JWT+admin)沿用 RequireJWTAdmin 模式扩展;**历史空 role 视为 admin 的行为保留** |
| API Token | `cpt_` Bearer,scope read/write 二值;Token 轨 UserID 注入 0(审计空) | 保留二值,映射到权限点(§3.3) |
| 登录防护 | LoginGuard 进程内锁定(settings 可调);按来源 IP 计数 | 不变 |
| 审计 | middleware/audit.go 唯一写入 | 新增用户/会话/2FA 事件类型 |

## 3. 角色与权限模型

### 3.1 权限点(权限字符串 = `<资源>:<操作>`)

| 权限点 | 说明 |
| --- | --- |
| `sites:read` / `sites:write` | 站点查看 / 增删改、启停、导入、证书上传 |
| `nodes:read` / `nodes:write` | 节点查看 / 接入、卸载、删除 |
| `config:read` | 查看当前配置/版本历史/diff |
| `config:rollback` | 回滚到历史版本(生成新版本下发) |
| `config:direct-edit` | **直接编辑 Caddyfile**(高危:可改全局块,见 §3.2 硬规则) |
| `logs:read` `metrics:read` `upstreams:read` | 日志 / 监控 / 上游状态查看 |
| `settings:read` / `settings:write` | 系统设置读 / 写(含通知渠道、Gravatar 镜像;备份导出/恢复已拆为独立权限点 backup:*) |
| `tokens:manage` | 开放 API Token 签发/删除 |
| `backup:export` / `backup:restore` | 备份导出 / 恢复 |
| `terminal:use` | Web 终端(高危,见 §3.2 硬规则) |
| `users:manage` | 用户管理(高危,见 §3.2 硬规则) |

> 仪表盘(`GET /status` 聚合)不单设权限点:登录用户可见,其数据跟随聚合来源的 `*:read`。
> `audit:read` **不在本期矩阵**:当前系统只有审计写入、无查看接口/页面,权限点不做无消费的空转条目;审计查看留待后续迭代,届时随接口一并加入。

### 3.2 内置角色(固定三种,不做自定义角色)

| 角色 | 权限 |
| --- | --- |
| `admin` | `*`(全部,含三个高危权限点) |
| `editor` | 除 `users:manage`、`terminal:use`、`config:direct-edit`、`settings:write`、`backup:restore`、`tokens:manage` 外的全部(sites/nodes 读写 + config:read/rollback + 各类只读 + backup:export) |
| `viewer` | 全部 `*:read` |

**硬规则**(代码断言,不入矩阵、任何角色组合不可绕过):

- `users:manage`、`terminal:use`、`config:direct-edit` 仅 admin(直编可改全局块含 admin 绑定,误操作影响面大,收归 admin 专属);viewer 永不可写(即使未来角色扩展,写操作权限点不派发给只读角色);
- 自我保护:不能禁用/删除/降级自己;系统至少保留一个可用 admin(最后一个 admin 不可禁用/降级/删除);
- **username 创建后不可修改**(登录名 + 人事锚点;需要变更时删旧建新,软删保留审计);admin 重置密码/改角色/禁用 → 目标用户全部会话吊销。

### 3.3 API Token 对齐

Token scope 保持二值(机器访问不需要角色):`read` → 全部 `*:read`;`write` → 全部读写,但**恒不含** `users:manage`、`terminal:use`、`config:direct-edit`(与 M3"Token 不能管理 Token"的边界原则一致,高危权限点禁止面从宽)。中间件将 scope 翻译为权限点集合后走同一条校验路径。**API Token 调用不受 2FA 约束**(2FA 是人类登录的第二因子,Token 是机器凭证且不经登录链路)。

### 3.4 角色 → 界面可见性(前端显隐依据,验收对照表)

| 菜单/入口 | admin | editor | viewer |
| --- | --- | --- | --- |
| 仪表盘 / 站点管理 / 上游状态 / 节点管理 / 日志 / 监控 | ✅ | ✅ | ✅ |
| 配置中心 | ✅(含直接编辑) | ✅ 只读预览 + 回滚(直接编辑入口隐藏) | ✅ 只读预览(回滚隐藏) |
| 终端 | ✅ | ❌ | ❌ |
| 设置页 | ✅(可编辑) | ✅ 只读(常规/备份可看,保存按钮隐藏;API Token Tab 隐藏) | ❌ 整页不可见 |
| 用户管理(新页) | ✅ | ❌ | ❌ |
| Header 个人设置(改密/会话/2FA) | ✅ | ✅ | ✅ |

> 设置页对 editor:有 `settings:read` 无 `settings:write` → 各 Tab 可见但只读;`backup:restore`/`tokens:manage` 两个 Tab 需对应权限点,editor 隐藏;`backup:export` editor 有 → 导出按钮可见。

## 4. 数据模型(AutoMigrate 增量)

### 4.1 users 表完整字段定义

| 字段 | 类型 | 约束/默认 | 说明 | 来源 |
| --- | --- | --- | --- | --- |
| `id` | uint | PK,自增 | 用户 ID(JWT `uid` claim) | M1 |
| `username` | string(64) | not null | 登录名,3–64 字符;唯一性见下方唯一索引说明 | M1 |
| `nickname` | string(64) | default 空 | 昵称(展示名);可空,界面回退显示 `username` | **M4** |
| `email` | string(255) | default 空 | 资料邮箱:Gravatar 头像来源;**仅资料字段,不做验证、不做找回**;非空时校验含 `@`;不做唯一约束 | **M4** |
| `avatar_path` | string(128) | default 空 | 自定义头像文件名(服务端生成,存 `data/avatars/`);空 = 回退 Gravatar;`json:"-"`,接口以 `avatar_url` 计算字段形式给出 | **M4** |
| `password_hash` | string(255) | **可空** | bcrypt 摘要;**任何接口不得外泄**(`json:"-"`);空 = 仅外部认证账号(未来 OIDC/LDAP 接入),不能走密码登录;本期全部创建路径仍必填 | M1,M4 放宽可空 |
| `role` | string(32) | not null,default `admin` | `admin`/`editor`/`viewer`(service 层白名单校验,非法值拒绝) | M1 值,M4 语义扩展 |
| `auth_source` | string(32) | not null,default `local` | 认证来源:`local` / 未来 `oidc`/`ldap`/`github`/`google` 等;**外部身份扩展预留字段**(本期恒为 local) | **M4** |
| `provider_subject` | string(128) | default 空 | 外部 provider 内的唯一主体 ID(OIDC `sub`/GitHub 数字 ID 等);**绑定锚点**,仅外部账号非空;唯一索引 `(auth_source, provider_subject) WHERE provider_subject != ''` | **M4** |
| `disabled` | bool | not null,default false | 禁用后拒绝登录;已有会话全部吊销 | **M4** |
| `must_change_password` | bool | not null,default false | admin 创建/重置密码时置 true;该用户登录响应带标记,前端强制改密弹窗(不可关),改密成功清标记 | **M4** |
| `totp_enabled` | bool | not null,default false | 二步验证开关(绑定确认后置 true) | **M4** |
| `totp_secret` | string(255) | default 空 | TOTP 密钥,**AES-256-GCM 密文**(密钥由 JWT 签名密钥派生);空 = 未绑定;`json:"-"` | **M4** |
| `totp_recovery` | string(1024) | default 空 | 10 个恢复码的 bcrypt 哈希(逗号分隔;单个 bcrypt 60 字符 × 10 + 分隔 ≈ 609,故需 1024);空 = 未生成;`json:"-"` | **M4** |
| `created_at` | time | auto | 创建时间(用户列表展示) | M1 |
| `updated_at` | time | auto | 更新时间 | **M4** |
| `deleted_at` | time | 可空,索引 | 软删除标记;非空即已删除,登录与全部查询排除 | **M4** |

#### username 唯一性(软删除语义下的关键细节)

- 普通 `uniqueIndex` 会与软删除冲突:已删用户永久占住用户名,无法重建同名账号;
- 因此采用 **SQLite 部分唯一索引**:`CREATE UNIQUE INDEX idx_users_username_alive ON users(username) WHERE deleted_at IS NULL`——仅存活用户间唯一,已删用户释放用户名;
- GORM 标签无法表达 partial index,由 store 层 AutoMigrate 之后执行幂等建索引语句(存在即跳过);
- 应用层创建时仍需先行查重,返回"用户名已存在"的业务错误(索引只是最后防线;username 创建后不可修改,见 §3.2);
- 审计与用户列表展示不依赖 username 唯一性(历史审计记录中的已删用户名可与后来者重名,按 user_id 区分)。

**API 可见性:** `username`/`nickname`/`email`/`role`/`disabled`/`totp_enabled`/`created_at` 可出现在接口响应;`password_hash`/`totp_secret`/`totp_recovery` 永不出接口(`json:"-"`),恢复码明文仅在启用 2FA 响应中出现一次。

**头像方案(自定义上传 > Gravatar 派生 > 首字母,三级回退):**

- **回退链**:有 `avatar_path` → 自定义上传头像;否则 email 非空 → Gravatar 派生;再否则 → 首字母圆形头像(取 `nickname || username` 首字符)。Gravatar 加载失败(离线/镜像不可达)同样回退首字母;
- **Gravatar 派生**:hash 用 `SHA-256(lower(trim(email)))`(Gravatar 新标准,避免 MD5 可逆推邮箱);URL = `<镜像 base>/avatar/<hash>?s=<size>&d=retro`,d=retro 保证任何邮箱都有图案兜底;
- **镜像站配置(防 gravatar.com 不可达)**:settings 键 `gravatar_mirror_url`(空 = 官方 `https://gravatar.com`),设置页"常规"Tab **可视化配置**——AutoComplete 预设快捷项 + 允许自定义,后端校验 http(s):// 开头(可空);预设:`https://gravatar.com`(官方)· `https://gravatar.qingtian.dev` · `https://cravatar.cn` · `https://weavatar.com` · `https://gravatar.loli.net`(第三方镜像可用性不受本项目控制,故允许自定义);镜像须兼容 Gravatar URL 协议(`/avatar/<hash>` 路径与 `d=` 参数);配置由前端经 `GET /settings`(全角色可读)获取后拼 URL,改镜像即时影响新渲染的头像;
- **上传**:仅本人可传(`POST /auth/avatar`,multipart);校验:类型白名单 png/jpeg/webp(以内容嗅探为准,不信扩展名)、≤ 2MiB;文件名服务端生成 `{uid}_{8位随机}.{ext}`(不可猜测),存 `data/avatars/`;替换上传时删除旧文件;自定义头像经本机 `/avatars/{file}` 提供,不依赖外网——**离线环境的可靠头像路径**;
- **访问**:`GET /avatars/{file}` 静态路由**不挂鉴权**(`<img>` 标签无法携带 JWT,与 Gravatar 公开 URL 同等敏感级);安全性靠不可猜文件名 + 仅图片内容;接口响应以计算字段 `avatar_url` 给出(上传者 `/avatars/..`,无上传者返回空,前端走 Gravatar/首字母);
- **移除**:`DELETE /auth/avatar` 删文件并清字段,回退 Gravatar;admin 不代管他人头像(个人资料自理);
- **展示位**:Header 用户菜单、用户管理列表、个人设置页;
- 展示名回退链:`nickname || username`。

#### 恢复码补全

启用 2FA 时发 10 个;恢复码可重新生成(`POST /auth/2fa/recovery`,需当前验证码)——旧码全部作废、发新 10 个,用于"恢复码快用完/怀疑泄露"场景;数量固定 10 个,不做增减。

### 4.2 sessions 表(新表)

```text
id            uint PK
user_id       uint idx
sid           string(64) uniqueIndex      -- 随机,写入 JWT sid claim
ip            string(64)
user_agent    string(255)
created_at / last_seen_at / expires_at
revoked_at    *time                       -- 非空即已吊销
```

- 迁移兼容:现有 role 值 `admin`/`viewer` 与新角色名一致,零迁移;AutoMigrate 加列默认值即可;
- **存量会话迁移**:M4 部署后,存量 JWT(无 `sid` claim)一律 401 —— 不提供兼容旁路(否则旧 token 绕过吊销机制,防线形同虚设)。代价是**上线瞬间全体在线用户被登出一次**(与轮换 JWT 密钥等效),TTL 默认 24h、影响窗口一天,验收时确认为已知行为;
- 会话有效期与 JWT TTL 同源(env),`last_seen_at` 节流更新(每分钟≤1 次);
- 清理:登录时顺手删除本人过期/吊销会话;启动时清扫全表过期会话。

## 5. 认证与会话流程

```text
登录:POST /auth/login(用户名+密码)
  ├─ 密码错(含用户名不存在,统一文案防枚举)→ 401(**计入 LoginGuard**)
  ├─ 禁用/已删除 → 401 明确文案(**不计入锁定**:账号状态问题非爆破,锁定无意义)
  ├─ 未启 2FA → 签发 JWT(claims: uid, role, sid),sessions 落一行,返回 token
  └─ 已启 2FA → 不发 token,返回 { require_2fa: true, login_ticket }(ticket:60s 一次性,内存)
        POST /auth/login/2fa(ticket + 6 位验证码或恢复码)
          ├─ 通过 → 同上签发 JWT(恢复码登录则烧掉该码;响应字段与直登一致:
          │   nickname / must_change_password 随附)
          └─ 失败 → 401(**计入 LoginGuard**,防止绕过密码后爆破 TOTP)

登出:POST /auth/logout(登录态)→ 吊销当前 sid。
  有会话表后"退出"必须是服务端动作:M1 起退出仅清前端 token,
  旧 token 到过期前仍有效、设备列表残留"已退出"会话 —— M4 一并修正。

首登强制改密:users.must_change_password(见 §4.1)。
  admin 创建用户设初始密码 → 该用户首次登录(或重置密码后首次登录)登录响应带
  must_change_password: true,前端弹**不可关闭**的改密弹窗,完成改密才进入面板;
  改密成功清标记。保证"admin 知道的初始密码"不会长期有效。

请求鉴权:JWT 签名校验 → sessions 查 sid(未吊销/未过期/未禁用/未删除用户)
  → 权限点 = roleToPermissions(claims.role) → 路由所需权限点比对
  会话查询每请求一次 DB(SQLite 本地,可接受;后续如需可在内存做 60s 微缓存)
  注:权限点只由 claims.role 推导,**不查库取角色** —— 角色变更的即时一致性
  由"角色变更必吊销该用户全部会话"保证(强制重登换取新 claims)。

强制改密的服务端强制(防绕过前端弹窗):session 检查时联读用户 `must_change_password`,
  为 true 的用户仅放行白名单端点 —— 写操作仅 change-password / profile / avatar / logout,
  只读放行 GET profile / sessions / settings(强制改密弹窗所在页面的渲染依赖),
  其余一律 403(code=password_change_required,前端据此定位到强制改密弹窗)。

吊销触发:改密 / admin 重置他人密码 / 禁用 / 角色变更 / 删除(软删) / 自助下线 / admin 强制下线
  → UPDATE sessions SET revoked_at WHERE user_id=? (或 sid=?)
```

JWT 仍为短签名 token;**吊销的即时性来自 session 检查**,JWT 本体过期时间不变(env 可调)。

### 5.1 外部身份源扩展预留(LDAP / OIDC / GitHub / Google)

> 本期**不实现**任何外部登录(见 §1.2),但以下预留随 M4 一并落地,保证未来接入是"增量插入"而非破坏性重构:

**字段层(本期落地,见 §4.1):**

- `auth_source`(default `local`)+ `provider_subject` + 唯一索引 `(auth_source, provider_subject) WHERE provider_subject != ''`——外部身份的绑定锚点,未来接入无需改表;
- `password_hash` 放宽可空——外部账号无本地密码(空 = 拒绝密码登录),本期创建路径仍全部必填。

**链路层(抽象约定,本期以 local 单实现落地):**

- service 层定义 **CredentialVerifier 接口**:`Verify(ctx, username, password) (*Identity, error)`,M4 提供 `localVerifier`(bcrypt)唯一实现;未来 LDAP = 新增一个实现(bind 校验),登录入口无需改动;
- OAuth 类(GitHub/Google/OIDC)是重定向流,不走 CredentialVerifier:未来新增 `GET /auth/oauth/{provider}/start` 与 `/auth/oauth/{provider}/callback` 两个路由,回调换取身份后**汇入与会话/2FA/审计相同的下游管线**(按 `(auth_source, provider_subject)` 查/建用户 → 签发 JWT → sessions 落行);
- 登录响应、JWT claims、会话表、权限点推导对外部用户**完全一致**——外部用户也是 role 驱动,权限矩阵零改动。

**未来接入时的决策(届时沿用,不在本期展开):**

- 账号供给策略(JIT 首登自动建号 / admin 预建 + 自助绑定)、初始角色(建议 default viewer + admin 提权)、email 匹配优先级——属 provider 配置,接入迭代时拍板;
- 外部用户与本地 TOTP:provider 自带 2FA 时面板侧 TOTP 叠加策略(默认不叠加)届时定;
- 头像:回退链可自然扩展一级"provider 头像"(OAuth 回调返回的 avatar_url),优先级介于自定义上传与 Gravatar 之间;
- 认证源管理 UI(启用 GitHub/OIDC、client_id/secret、LDAP 地址端口)为设置页新 Tab + 新 settings 键,接入迭代时设计;
- 审计:auth.login 事件的 detail 天然容纳 `source` 维度,无需改表。

**演进定位**:外部认证列为 M4 之后的候选里程碑(M5 候选,优先级由用户届时定);LDAP 适合企业内网,GitHub/Google/OIDC 适合团队 SSO。

## 6. TOTP 两步验证

- 库:**pquerna/otp**(Go 生态标准选择,零重依赖);
- 绑定:`POST /auth/2fa/setup` 生成 secret + `otpauth://` URI(issuer=Caddy Panel,user=用户名)→ 前端渲染二维码(轻量 QR 库)→ 用户输码 `POST /auth/2fa/enable` 校验通过才落库启用,返回 10 个一次性恢复码(明文仅此一次);
- 解绑:`DELETE /auth/2fa`(需当前密码 + 当前验证码,防劫持会话关闭 2FA);解绑吊销全部会话;
- 登录:验证码 30s 窗口 ±1 容错;恢复码 bcrypt 比对,命中即烧(从哈希列表移除);
- 密钥加密:AES-256-GCM,key = SHA-256(jwt 签名密钥 + "totp")派生 —— 密钥本就持久化在 settings 表,同机威胁模型下属纵深防御;
- 本期不做:全局强制 2FA 开关(设计预留 settings 键,后续迭代加 UI 即可);
- **已知限制(V19 验收确认)**:2FA 错码计入 LoginGuard,但攻击者"正确密码 → 新票据 → 错码"循环会因密码成功而重置计数——TOTP 30s 窗口 ±1 仅 3 个有效码,在线爆破单次成功率约 0.0003%,实际风险可接受;强化方案(独立 2FA 失败计数)列入后续迭代。

## 7. API 设计(全部 swaggo 注解;错误统一 `{code,message,detail}`)

| 方法/路径 | 权限 | 说明 |
| --- | --- | --- |
| `GET /api/v1/users` | users:manage | 用户列表(含会话数) |
| `POST /api/v1/users` | users:manage | 创建(用户名+初始密码+角色) |
| `PATCH /api/v1/users/:id` | users:manage | 改角色/禁用启用/重置密码(触发吊销) |
| `DELETE /api/v1/users/:id` | users:manage | **软删除**(保留审计 user_id 可追溯)+ 吊销会话 |
| `GET /api/v1/users/:id/sessions` | users:manage | 该用户会话列表(admin 排查视角,自查走 /auth/sessions) |
| `DELETE /api/v1/users/:id/sessions/:sid` | users:manage | admin 强制下线该用户指定会话 |
| `GET /api/v1/auth/profile` | 登录态 | 本人资料读取(nickname/email/avatar_url/role/username;个人设置页与 Header 展示数据源) |
| `PATCH /api/v1/auth/profile` | 登录态 | 自助维护昵称/邮箱(不涉及角色与凭证,不触发吊销) |
| `POST /api/v1/auth/avatar` | 登录态 | 上传自定义头像(multipart;png/jpeg/webp 内容嗅探,≤2MiB;替换时删旧文件) |
| `DELETE /api/v1/auth/avatar` | 登录态 | 移除自定义头像(删文件清字段,回退 Gravatar) |
| `POST /api/v1/auth/logout` | 登录态 | 退出登录:吊销当前会话(修正 M1 起纯前端退出的残留有效 token) |
| `POST /api/v1/auth/change-password` | 登录态 | 旧密码 + 新密码(≥8 字符且 ≠ 旧密码);成功清 `must_change_password` 并吊销本人全部会话(含当前,响应提示重登) |
| `GET /api/v1/auth/sessions` | 登录态 | 本人会话列表(当前标记) |
| `DELETE /api/v1/auth/sessions/:sid` | 登录态 | 本人下线指定会话 |
| `POST /api/v1/auth/sessions/others` | 登录态 | 一键下线除当前设备外的全部会话(个人设置页便捷操作) |
| `POST /api/v1/auth/2fa/setup` / `enable` / `DELETE` | 登录态 | 绑定/确认启用/解绑 |
| `POST /api/v1/auth/2fa/recovery` | 登录态 + 当前验证码 | 重新生成 10 个恢复码(旧码全部作废) |
| `POST /api/v1/auth/login` / `login/2fa` | 公开 | 如 §5;登录响应附 `must_change_password` |

角色变更、禁用、删除、重置密码、2FA 启停全部写审计(操作者/目标/结果)。

## 8. 前端改动

| 位置 | 内容 |
| --- | --- |
| 新页 `pages/Users`(菜单"用户管理",仅 admin 可见) | 用户表格 + 新建/编辑 Drawer(角色下拉/重置密码/禁用开关 + 保护规则错误透出);行内展开会话列表 + 强制下线 |
| Header 用户下拉扩展 | "个人设置"(资料:昵称/邮箱 + 头像预览;改密/会话管理/2FA 绑定,Drawer 或独立页);展示名与头像按 §4.1 回退链渲染 |
| 登录响应 | 附带 `nickname`(前端展示名回退 `username`);JWT claims 保持最小(uid/role/sid),不塞资料字段 |
| 登录页 | 2FA 验证码第二步(检测 `require_2fa` 切换输入,含恢复码入口) |
| 权限可见性 | 菜单/按钮按 §3.4 可见性表显隐(现有 isAdmin 判断改造为 `hasPermission('...')` hook) |
| i18n | zh/en 同步新增全部文案(铁律 6) |

### 8.1 个人设置页功能清单(`pages/Profile`,全部角色可用)

> 入口:Header 用户下拉"个人设置"。页面分四个区块,全部本人操作,不需要 `users:manage`。

#### ① 资料区

- 头像:当前头像预览(按 §4.1 回退链渲染)+ 上传按钮(选图即传)+ "移除自定义头像"(有上传时显示);
- 昵称(可空,placeholder 提示"留空显示用户名")、邮箱(可空,email 格式前端预校验);保存按钮 → `PATCH /auth/profile`,成功后 Header 展示名与头像即时刷新;
- 只读展示:用户名、角色(不可自助修改;用户名是登录凭证,角色属管理域)。

#### ② 安全区 — 修改密码

- 旧密码 + 新密码 + 确认新密码;新密码 ≥8 字符且不得与旧密码相同(前后端同校验,**经用户确认不做字符组合复杂度要求**,强度条仅提示);
- 成功后 toast 提示"密码已修改,请重新登录" → 清本地会话跳登录页(全部会话已吊销,`must_change_password` 同步清除);
- **强制改密弹窗**:登录响应带 `must_change_password: true` 时,先于一切页面弹出不可关闭的改密弹窗(仅旧密码+新密码+确认),完成后进入面板 —— 覆盖 admin 创建账号与重置密码两个场景。

#### ③ 安全区 — 两步验证(2FA)

- 未启用:说明文案 + "启用两步验证" → 弹窗展示二维码(otpauth URI)→ 输入 6 位验证码确认 → **展示 10 个恢复码(仅此一次,提供复制)** → 启用;
- 已启用:状态徽标 + "重新生成恢复码"(需当前验证码,旧码作废)+ "解绑"(需当前密码 + 当前验证码,双因子确认);
- 登录二段、恢复码消耗逻辑见 §5/§6。

#### ④ 安全区 — 登录设备(会话管理)

- 会话列表:设备(UA 解析出浏览器/系统简称)、IP、最后活跃、创建时间;当前设备置顶并标"本设备";
- 每行"下线"按钮(当前设备除外,文案提示下线后需重新登录);
- "下线其他设备"总按钮(`POST /auth/sessions/others`),二次确认;
- 2FA/会话/改密等安全操作全部写入审计。

> 用户管理页(admin)同步获得两个轻量信息:列表加"最后活跃"列(取该用户全部会话的最大 `last_seen_at`,无会话显示"—"),行内展开会话列表 + 强制下线。

## 9. 任务拆解

| ID | 任务 | 依赖 |
| --- | --- | --- |
| T4.1 | sessions 表 + JWT sid 双检 + 吊销 API(含 logout)+ 会话清扫 + 外部认证预留字段(auth_source/provider_subject/password_hash 可空,CredentialVerifier 抽象) | — |
| T4.2 | 权限点模型(roleToPermissions + 中间件改造 + Token scope 对齐)+ 三角色迁移 | T4.1 |
| T4.3 | 用户管理 CRUD API(软删/保护规则/must_change_password 及其服务端强制/审计)+ 前端用户管理页(最后活跃列/会话强制下线) | T4.2 |
| T4.4 | 自助链路:个人资料(昵称/邮箱/头像上传)+ 改密(策略校验)+ 会话列表/下线/下线其他 + Header 个人设置 + 首登强制改密弹窗 + Gravatar 镜像设置(settings 键 gravatar_mirror_url 入白名单/校验 + 设置页常规 Tab AutoComplete 字段) | T4.3 |
| T4.5 | TOTP 2FA 全链路(绑定/登录两段/恢复码 + 重新生成/解绑) | T4.4 |
| T4.6 | 真实验收:按 §10 验收流程执行(S1 自测 → compose 部署 → V0–V28 用例 → 缺陷清零 → 记录归档) | 全部 |

每任务按开发规范执行:失败分支单测、swaggo 注解、i18n 双语、pipeline 边界不越。

## 10. 验收流程与用例

> 里程碑验收沿用项目铁律:**真实验收,不做全 mock 验收**(M2/M3 的生产缺陷均由真实联调拦截)。
> 验收负责人:开发会话(实现者);验收记录写入 [progress.md](./progress.md) 决策日志,验收期缺陷当场修复并复验后留痕。

### 10.1 验收环境与准备

- **环境**:验收服务器 `https://caddy-panel.realqingtian.com`(独立 Linux 主机,Caddy 边缘反代 + 自动 HTTPS;连接信息与部署约定见项目本地 `ops/server.md`,该目录不入库);本地 compose 为备选/调试环境;
- **账号矩阵**(验收前由 admin 在 UI 创建):admin(首启 seed)/ editor(初始密码)/ viewer(初始密码)——editor/viewer 的首登即覆盖强制改密用例;
- **TOTP 验证码生成**:验收机 `oathtool -b --totp <secret>`(或浏览器 Authenticator 插件导入 otpauth URI);
- **前置数据**:1 个反代站点(site-a,指向可用上游)、1 个 `cpt_` Token(write)用于 FR-905 回归;设置页 `panel_public_url` 配置为验收域名。

### 10.2 验收阶段

| 阶段 | 内容 | 通过条件 |
| --- | --- | --- |
| S1 开发自测 | 每任务完成即跑:service 单测(失败分支)、`go vet ./... && go test ./...`、`bun run lint && bun run build` | 全绿 |
| S2 部署 | GH Actions 构建镜像推 GHCR(`docker.yml`,amd64+arm64)→ 验收服务器 `docker compose pull && up -d`(域名 + 自动 HTTPS);适用时执行存量会话用例 V16(升级部署场景) | — |
| S3 用例执行 | 按 §10.3 清单逐条浏览器实操 + 必要时 curl 复核 API 层,逐条记录 ✅/缺陷 | 全部 ✅ |
| S4 缺陷处理 | 缺陷分级(阻塞/一般/轻微);**阻塞项修复并复验通过才算验收通过**;全部缺陷与修复留痕 | 阻塞项清零 |
| S5 记录归档 | 验收结论(含缺陷清单与修复说明)写入 progress 决策日志;M4 设计文档按惯例归档 docs/archive/ | 完成即 M4 关闭 |

### 10.3 验收用例清单

#### 准备(V0)

| # | 用例 |
| --- | --- |
| V0 | compose 起;admin 登录;创建 site-a 反代站点;创建 editor/viewer(初始密码) |

#### FR-901 用户管理(V1–V7)

| # | 用例 | 对应设计 |
| --- | --- | --- |
| V1 | admin 创建用户 → 列表可见(昵称/角色/状态/最后活跃列) | §7 users |
| V2 | editor 首登 → **不可关闭**改密弹窗 → 改密进入面板;再登不弹 | §8.1 ② |
| V3 | admin 重置密码 → 用户全部会话 401 → 重登触发强制改密 | §5 吊销触发 |
| V4 | 禁用用户 → 立即 401;启用恢复;禁用后菜单/接口全不可达 | §4.1 disabled |
| V5 | 保护规则:admin 对自己禁用/降级/删除均 400;禁用最后一个 admin 400 | §3.2 |
| V6 | 删除用户 → 无法登录;同用户名可重建;审计 user_id 保留 | §4.1 软删 |
| V7 | admin 查看任意用户会话列表 + 强制下线指定会话 | §7 users/:id/sessions |

#### FR-902 权限(V8–V10)

| # | 用例 | 对应设计 |
| --- | --- | --- |
| V8 | editor:菜单符合 §3.4;站点/节点可管理;回滚可用;**直接编辑入口隐藏且直编 API 403** | §3.2/§3.4 |
| V9 | viewer:全只读;写 API 均 403;设置页/终端不可见 | §3.4 |
| V10 | 越权 403 文案 zh/en 切换均本地化 | §3.1 注 |

#### FR-903 会话(V11–V16)

| # | 用例 | 对应设计 |
| --- | --- | --- |
| V11 | 同一用户双浏览器登录 → 设备列表两条,当前标"本设备" | §8.1 ④ |
| V12 | 下线单条会话 → 对应设备下一次请求 401 | §7 sessions/:sid |
| V13 | "下线其他设备" → 其余全失效,当前保持在线 | §7 sessions/others |
| V14 | 退出登录 → 原 token 下一次请求 401(服务端吊销) | §7 logout |
| V15 | 改密 → 全部会话(含当前)401,前端跳登录;新密码 <8 或与旧密码相同被拒 | §7 change-password |
| V16 | 存量兼容:含旧 JWT 的部署升级 M4 后,旧 token 一律 401(**仅升级部署场景适用;全新部署跳过**) | §4 迁移 |

#### FR-904 两步验证(V17–V22)

| # | 用例 | 对应设计 |
| --- | --- | --- |
| V17 | 绑定:扫码 → oathtool 出码确认 → 恢复码仅此一次展示 | §6 |
| V18 | 登录二段:仅密码 → require_2fa;密码 + 验证码 → 成功 | §5 |
| V19 | 验证码连续错误达阈值 → 账号锁定(LoginGuard) | §5 |
| V20 | 恢复码登录成功且烧毁;重新生成后旧码全部失效 | §6 |
| V21 | 解绑(密码 + 验证码)→ 登录不再二段;会话吊销 | §6 |
| V22 | 2FA 启用用户照常签发/使用 API Token(Token 不受 2FA 约束) | §3.3 |

#### FR-905/906 兼容与预留(V23–V24)

| # | 用例 | 对应设计 |
| --- | --- | --- |
| V23 | `cpt_` Token read/write 行为与 M3 一致;Token 访问 users 域 403 | §3.3 |
| V24 | users 表含 auth_source(=local)/provider_subject(空);本地全流程回归通过 | §5.1 |

#### 横切(V25–V28)

| # | 用例 | 对应设计 |
| --- | --- | --- |
| V25 | 头像:上传(含非图片/超限拒绝)、移除、镜像切换、首字母回退 | §4.1 头像方案 |
| V26 | 审计:创建/禁用/删除/重置/2FA 启停/强制下线事件均落 audit_logs | §7 注 |
| V27 | 新增页面与文案 zh/en 完整(check-i18n 脚本零缺失) | 铁律 6 |
| V28 | 回归:站点/配置/日志/监控/终端(仅 admin)在权限点改造后行为与 M3 一致 | FR-905 |

> 未改密服务端强制(403 password_change_required)、直编仅 admin 等 §3.2/§5 的硬规则,在 V2/V8/V9 中随场景覆盖。

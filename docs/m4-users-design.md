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

- 开放注册、邮箱验证、自助找回密码(自托管场景:管理员重置);
- 自定义角色 / 资源级 ACL(内置角色 + 权限点矩阵已覆盖面板场景,自定义角色属过度设计;矩阵设计保证未来加角色只是加一行映射);
- OIDC/OAuth/LDAP 外接(路线 C/B 被否;本设计的权限点与中间件分层为其预留了接入位,未来需要时在登录链路加 provider 分支即可);
- passkey/WebAuthn。

## 2. 现状基线(M3 末)

| 项 | 现状 | 本期动作 |
| --- | --- | --- |
| users 表 | id/username/password_hash(bcrypt)/role/created_at,仅 seed 单 admin | 加列 + 管理接口 |
| 认证 | 无状态 JWT(签名密钥持久化 settings,有效期 env `PANEL_JWT_TTL`) | JWT 加 `sid` claim,与会话表双检 |
| RBAC | role 两级 admin/viewer,路由级 scope(read/write) | 升级为权限点矩阵(见 §3) |
| API Token | `cpt_` Bearer,scope read/write 二值 | 保留二值,映射到权限点(§3.3) |
| 登录防护 | LoginGuard 进程内锁定(settings 可调) | 不变 |
| 审计 | middleware/audit.go 唯一写入 | 新增用户/会话/2FA 事件类型 |

## 3. 角色与权限模型

### 3.1 权限点(权限字符串 = `<资源>:<操作>`)

| 权限点 | 说明 |
| --- | --- |
| `sites:read` / `sites:write` | 站点查看 / 增删改、启停、导入、证书上传 |
| `nodes:read` / `nodes:write` | 节点查看 / 接入、卸载、删除 |
| `config:read` / `config:write` | 配置查看 / 直编、回滚、下发 |
| `logs:read` `metrics:read` `upstreams:read` | 日志 / 监控 / 上游状态查看 |
| `settings:read` / `settings:write` | 系统设置读 / 写(含备份导出导入、通知渠道) |
| `tokens:manage` | 开放 API Token 签发/删除 |
| `backup:export` / `backup:restore` | 备份导出 / 恢复 |
| `audit:read` | 审计日志查看 |
| `terminal:use` | Web 终端(高危,见 §3.2 硬规则) |
| `users:manage` | 用户管理(高危,见 §3.2 硬规则) |

### 3.2 内置角色(固定三种,不做自定义角色)

| 角色 | 权限 |
| --- | --- |
| `admin` | `*`(全部,含两个高危权限点) |
| `editor` | 除 `users:manage`、`terminal:use`、`settings:write`、`backup:restore`、`tokens:manage` 外的全部(sites/nodes/config 读写 + 各类只读 + backup:export) |
| `viewer` | 全部 `*:read` |

**硬规则**(代码断言,不入矩阵、任何角色组合不可绕过):

- `users:manage`、`terminal:use` 仅 admin;viewer 永不可写(即使未来角色扩展,写操作权限点不派发给只读角色);
- 自我保护:不能禁用/删除/降级自己;系统至少保留一个可用 admin(最后一个 admin 不可禁用/降级/删除);
- admin 修改他人密码/角色/禁用状态 → 目标用户全部会话吊销。

### 3.3 API Token 对齐

Token scope 保持二值(机器访问不需要角色):`read` → 全部 `*:read`;`write` → 全部读写,但**恒不含** `users:manage`、`terminal:use`(与 M3"Token 不能管理 Token"的边界原则一致,禁止面扩大)。中间件将 scope 翻译为权限点集合后走同一条校验路径。

## 4. 数据模型(AutoMigrate 增量)

```
users(改)
  + disabled        bool   default false   -- 禁用后拒绝登录
  + totp_enabled    bool   default false
  + totp_secret     string(255)            -- AES-GCM 密文(密钥由 JWT 签名密钥派生),空=未绑定
  + totp_recovery   string(512)            -- 恢复码 bcrypt 串(逗号分隔哈希),空=未生成

sessions(新表)
  id            uint PK
  user_id       uint idx
  sid           string(64) uniqueIndex      -- 随机,写入 JWT sid claim
  ip            string(64)
  user_agent    string(255)
  created_at / last_seen_at / expires_at
  revoked_at    *time                       -- 非空即已吊销
```

- 迁移兼容:现有 role 值 `admin`/`viewer` 与新角色名一致,零迁移;AutoMigrate 加列默认值即可;
- 会话有效期与 JWT TTL 同源(env),`last_seen_at` 节流更新(每分钟≤1 次);
- 清理:登录时顺手删除本人过期/吊销会话;启动时清扫全表过期会话。

## 5. 认证与会话流程

```
登录:POST /auth/login(用户名+密码)
  ├─ 密码错/禁用 → 401(LoginGuard 计数不变)
  ├─ 未启 2FA → 签发 JWT(claims: uid, role, sid),sessions 落一行,返回 token
  └─ 已启 2FA → 不发 token,返回 { require_2fa: true, login_ticket }(ticket:60s 一次性,内存)
        POST /auth/login/2fa(ticket + 6 位验证码或恢复码)
          ├─ 通过 → 同上签发 JWT(恢复码登录则烧掉该码)
          └─ 失败 → 401(计入 LoginGuard)

请求鉴权:JWT 签名校验 → sessions 查 sid(未吊销/未过期/未禁用用户)
  → 权限点 = roleToPermissions(claims.role) → 路由所需权限点比对
  会话查询每请求一次 DB(SQLite 本地,可接受;后续如需可在内存做 60s 微缓存)

吊销触发:改密 / admin 重置他人密码 / 禁用 / 角色变更 / 自助下线 / admin 强制下线
  → UPDATE sessions SET revoked_at WHERE user_id=? (或 sid=?)
```

JWT 仍为短签名 token;**吊销的即时性来自 session 检查**,JWT 本体过期时间不变(env 可调)。

## 6. TOTP 两步验证

- 库:**pquerna/otp**(Go 生态标准选择,零重依赖);
- 绑定:`POST /auth/2fa/setup` 生成 secret + `otpauth://` URI(issuer=Caddy Panel,user=用户名)→ 前端渲染二维码(轻量 QR 库)→ 用户输码 `POST /auth/2fa/enable` 校验通过才落库启用,返回 10 个一次性恢复码(明文仅此一次);
- 解绑:`DELETE /auth/2fa`(需当前密码 + 当前验证码,防劫持会话关闭 2FA);解绑吊销全部会话;
- 登录:验证码 30s 窗口 ±1 容错;恢复码 bcrypt 比对,命中即烧(从哈希列表移除);
- 密钥加密:AES-256-GCM,key = SHA-256(jwt 签名密钥 + "totp")派生 —— 密钥本就持久化在 settings 表,同机威胁模型下属纵深防御;
- 本期不做:全局强制 2FA 开关(设计预留 settings 键,后续迭代加 UI 即可)。

## 7. API 设计(全部 swaggo 注解;错误统一 `{code,message,detail}`)

| 方法/路径 | 权限 | 说明 |
| --- | --- | --- |
| `GET /api/v1/users` | users:manage | 用户列表(含会话数) |
| `POST /api/v1/users` | users:manage | 创建(用户名+初始密码+角色) |
| `PATCH /api/v1/users/:id` | users:manage | 改角色/禁用启用/重置密码(触发吊销) |
| `DELETE /api/v1/users/:id` | users:manage | 删除(有 API Token/站点归属不受影响,审计留痕) |
| `POST /api/v1/auth/change-password` | 登录态 | 旧密码 + 新密码;成功即吊销本人全部会话(含当前,响应提示重登) |
| `GET /api/v1/auth/sessions` | 登录态 | 本人会话列表(当前标记) |
| `DELETE /api/v1/auth/sessions/:sid` | 登录态 | 本人下线指定会话 |
| `POST /api/v1/auth/2fa/setup` / `enable` / `DELETE` | 登录态 | 绑定/确认启用/解绑 |
| `POST /api/v1/auth/login` / `login/2fa` | 公开 | 如 §5 |

角色变更、禁用、删除、重置密码、2FA 启停全部写审计(操作者/目标/结果)。

## 8. 前端改动

| 位置 | 内容 |
| --- | --- |
| 新页 `pages/Users`(菜单"用户管理",仅 admin 可见) | 用户表格 + 新建/编辑 Drawer(角色下拉/重置密码/禁用开关 + 保护规则错误透出) |
| Header 用户下拉扩展 | "个人设置"(改密/会话管理/2FA 绑定,Drawer 或独立页)|
| 登录页 | 2FA 验证码第二步(检测 `require_2fa` 切换输入,含恢复码入口) |
| 权限可见性 | 菜单/按钮按权限点显隐(现有 isAdmin 判断改造为 `hasPermission('...')` hook) |
| i18n | zh/en 同步新增全部文案(铁律 6) |

## 9. 任务拆解

| ID | 任务 | 依赖 |
| --- | --- | --- |
| T4.1 | sessions 表 + JWT sid 双检 + 吊销 API + 会话清扫 | — |
| T4.2 | 权限点模型(roleToPermissions + 中间件改造 + Token scope 对齐)+ 三角色迁移 | T4.1 |
| T4.3 | 用户管理 CRUD API(保护规则 + 审计)+ 前端用户管理页 | T4.2 |
| T4.4 | 自助链路:改密 + 会话列表/下线 + Header 个人设置 | T4.3 |
| T4.5 | TOTP 2FA 全链路(绑定/登录两段/恢复码/解绑) | T4.4 |
| T4.6 | 真实验收:compose 全链路(多角色登录/权限矩阵逐点验证/踢会话/2FA 登录/API Token 兼容回归) | 全部 |

每任务按开发规范执行:失败分支单测、swaggo 注解、i18n 双语、pipeline 边界不越。

## 10. 验收标准(节选)

- admin 创建 editor/viewer → 各自登录仅见授权菜单;越权 API 返回 403;
- 改密后,其他设备会话在下一次请求即 401;
- 禁用用户 → 该用户立即无法访问;最后一个 admin 不可禁用(400 提示);
- 绑定 TOTP 后,仅凭密码登录返回 `require_2fa`;恢复码登录成功且烧毁;
- 旧 `cpt_` Token read/write 行为与 M3 验收一致(回归);
- 现有全部功能回归通过(权限点翻译不改变既有 admin 行为)。

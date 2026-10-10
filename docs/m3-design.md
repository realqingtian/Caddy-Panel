# Caddy Panel — M3 详细设计

> 前置:[PRD 第 5 节(M3 占位)](./requirements.md) · [M2 设计](./m2-design.md)
> M2 已预留的演进边界继续生效:Client 方法签名不写死 host、sites/config_versions 预留 node_id 列(AutoMigrate + 一次性回填 local)、单实例约束。

## 1. 需求与任务拆解(顺序即执行顺序)

| ID | 需求 | 要点 | 风险 |
| --- | --- | --- | --- |
| T3.1 | 开放 API Token + RBAC | 新表 api_tokens(name/token 哈希/scopes/过期);认证中间件接受 `Authorization: Bearer <token>`(与 JWT 并存);scopes:read/write 按路由组声明;RBAC:users.role 扩展 viewer(只读路由)与 admin(全部),登录 JWT 载入 role 并在中间件校验 | 权限粒度设计错误会静默放行 —— 路由级白名单 + 测试全覆盖 |
| T3.2 | Web 终端 | WebSocket(Gin + coder/websocket)+ xterm.js;后端 PTY 执行 `/bin/sh`(仅 admin 角色可见);会话随 WS 关闭终止;审计记录"打开终端"事件 | 宿主非容器时即主机 shell —— 文档必须警示;禁止 root 强制降权提示 |
| T3.3 | 多节点 Agent | 独立 `agent/` 二进制(反向连接中心,出站 WebSocket + 心跳,免暴露被管机端口);中心按 node 路由 Admin API 调用与 Caddyfile 下发;sites/config_versions/audit_logs 增加 node_id(回填 local);UI:节点列表 + 全站按节点筛选(Dashboard/Sites/配置中心)。**接入认证:Enrollment Token(一次性注册令牌,注册后签发节点密钥);安装:面板生成安装命令(下载二进制 + systemd unit)—— 均已拍板** | 协议设计、断线重连、并发下发跨节点锁;M3 主体工作量 |
| T3.4 | l4 转发 | caddy-l4 插件(compose 镜像追加);站点类型扩展 `l4`(监听地址端口/上游地址端口);**范围已拍板:仅 TCP/UDP 透传,不做 l4 层 TLS 终结**;与 http 站点渲染互斥 | l4 语法细节实现时对照 caddy-l4 文档 |

## 2. 数据模型增量(设计定稿后细化)

```
api_tokens
  id, name, token_hash(sha256,展示仅前缀), scopes(TEXT: read|write),
  last_used_at, expires_at(nullable), created_at

users.role  取值扩展:admin | viewer(M1 已预留 role 字段)

sites / config_versions / audit_logs
  node_id(TEXT,default "local";T3.3 引入时 AutoMigrate + 回填)

nodes(T3.3)
  id, name, public_key, last_seen_at, version, status
```

## 3. 关键决策(先行拍板;2026-10-11 用户确认四项见 §3.1)

1. **Agent 反向连接**:被管节点主动连中心(出站 WSS),中心不暴露节点端口 —— 与 NFR-101"零侵入/最小暴露"一致。
2. **Token 只存哈希**:展示一次性明文,丢失只能重建;`last_used_at` 供轮换。
3. **RBAC 不引入独立权限表**:M3 仅 admin/viewer 两级,路由组声明所需 scope,中间件统一校验;M4 若需细粒度再演进。
4. **终端不开 root**:Web 终端以 panel 进程用户执行;文档标注容器部署时建议 `--user` 非 root。

## 3.2 本地性与多节点语义(开工前审查,2026-10-11 用户确认)

总原则:**agent 自管本地,中心管策略**。七点澄清(均为该原则的推论,已确认):

1. 路径类设置(Caddyfile/日志/证书存储路径)仅对 local 节点生效;agent 节点的本地路径由 agent 自行配置(启动参数),不经中心 settings 下发。
2. 手动上传证书、Web 终端、Caddyfile 导入均限定 local 节点;远程节点的证书走 ACME/DNS challenge(渲染于该节点站点)。
3. Caddyfile 渲染按节点分组:每次下发的渲染输入 = 该节点的 enabled 站点(不再是全局全量)。
4. 下发并发语义演进:每节点串行(各自单写者锁)、跨节点并行。
5. 漂移检测由 agent 在本机执行并上报 in_sync 状态,中心聚合展示(Dashboard 按节点)。
6. metrics 样本与上游状态由 agent 上报(样本带 node_id),中心存储与图表按节点维度展示。
7. 站点归属节点在创建时确定(默认 local),**不支持迁移** —— 换节点 = 删除重建;回滚按节点:config_versions 带 node_id,回滚仅影响该节点;备份/恢复为全中心语义(含全部节点数据)。

## 3.1 T3.3 补充要点(审查补遗 + 用户拍板)

- **接入认证(拍板):Enrollment Token**。流程:面板生成一次性注册令牌(可设过期)→ agent 携令牌首连注册 → 中心创建 node 记录并签发节点密钥(node_secret,哈希存储)→ 后续心跳/指令均凭节点密钥;令牌用后作废,可撤销。
- **安装形态(拍板):面板生成安装命令**。节点管理页输出一条命令(下载对应平台二进制 + 写入 systemd unit + 内嵌注册令牌),目标机粘贴执行即完成接入;提供卸载命令对称生成。
- **平台矩阵**:linux amd64 / arm64(开发环境 macOS 可运行)。

- **Agent 发现本机 Caddy**:默认 `unix:///run/caddy/admin.sock`,退回 `http://localhost:2019`;agent 启动参数可覆盖 —— 与中心面板的 PANEL_ADMIN_URL 语义一致。
- **跨节点 UI**:Dashboard 状态卡与站点列表增加节点维度(节点选择器);audit_logs 按 node_id 过滤。
- **中心宕机行为**:agent 断线缓存待下发指令?不 —— agent 仅执行中心实时指令,断线即只读上报心跳;恢复后中心重新拉取节点状态(避免指令重放歧义)。

## 4. 插件依赖

| 功能 | 插件 | 镜像 |
| --- | --- | --- |
| T3.4 l4 | github.com/mholt/caddy-l4 | compose caddy Dockerfile 追加 --with(T3.4 落地时) |

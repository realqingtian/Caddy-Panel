# Caddy Panel — M3 详细设计

> 前置:[PRD 第 5 节(M3 占位)](./requirements.md) · [M2 设计](./m2-design.md)
> M2 已预留的演进边界继续生效:Client 方法签名不写死 host、sites/config_versions 预留 node_id 列(AutoMigrate + 一次性回填 local)、单实例约束。

## 1. 需求与任务拆解(顺序即执行顺序)

| ID | 需求 | 要点 | 风险 |
| --- | --- | --- | --- |
| T3.1 | 开放 API Token + RBAC | 新表 api_tokens(name/token 哈希/scopes/过期);认证中间件接受 `Authorization: Bearer <token>`(与 JWT 并存);scopes:read/write 按路由组声明;RBAC:users.role 扩展 viewer(只读路由)与 admin(全部),登录 JWT 载入 role 并在中间件校验 | 权限粒度设计错误会静默放行 —— 路由级白名单 + 测试全覆盖 |
| T3.2 | Web 终端 | WebSocket(Gin + coder/websocket)+ xterm.js;后端 PTY 执行 `/bin/sh`(仅 admin 角色可见);会话随 WS 关闭终止;审计记录"打开终端"事件 | 宿主非容器时即主机 shell —— 文档必须警示;禁止 root 强制降权提示 |
| T3.3 | 多节点 Agent | 独立 `agent/` 二进制(反向连接中心,出站 WebSocket + 心跳,免暴露被管机端口);中心按 node 路由 Admin API 调用与 Caddyfile 下发;sites/config_versions/audit_logs 增加 node_id(回填 local);UI:节点列表 + 全站按节点筛选(Dashboard/Sites/配置中心) | 协议设计、断线重连、并发下发跨节点锁;M3 主体工作量 |
| T3.4 | l4 转发 | caddy-l4 插件(compose 镜像追加);站点类型扩展 `l4`(地址/端口/上游);渲染走 l4 层语法 | l4 与 http 站点渲染互斥;插件镜像变体需含 caddy-l4 |

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

## 3. 关键决策(先行拍板)

1. **Agent 反向连接**:被管节点主动连中心(出站 WSS),中心不暴露节点端口 —— 与 NFR-101"零侵入/最小暴露"一致。
2. **Token 只存哈希**:展示一次性明文,丢失只能重建;`last_used_at` 供轮换。
3. **RBAC 不引入独立权限表**:M3 仅 admin/viewer 两级,路由组声明所需 scope,中间件统一校验;M4 若需细粒度再演进。
4. **终端不开 root**:Web 终端以 panel 进程用户执行;文档标注容器部署时建议 `--user` 非 root。

## 3.1 T3.3 补充要点(审查补遗)

- **Agent 发现本机 Caddy**:默认 `unix:///run/caddy/admin.sock`,退回 `http://localhost:2019`;agent 启动参数可覆盖 —— 与中心面板的 PANEL_ADMIN_URL 语义一致。
- **跨节点 UI**:Dashboard 状态卡与站点列表增加节点维度(节点选择器);audit_logs 按 node_id 过滤。
- **中心宕机行为**:agent 断线缓存待下发指令?不 —— agent 仅执行中心实时指令,断线即只读上报心跳;恢复后中心重新拉取节点状态(避免指令重放歧义)。

## 4. 插件依赖

| 功能 | 插件 | 镜像 |
| --- | --- | --- |
| T3.4 l4 | github.com/mholt/caddy-l4 | compose caddy Dockerfile 追加 --with(T3.4 落地时) |

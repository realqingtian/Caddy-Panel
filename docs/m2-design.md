# Caddy Panel — M2 详细设计

> 前置:[PRD 第 5 节(M2 需求)](./requirements.md) · [M1 设计](./m1-design.md) · [调研:插件与 metrics 预研](./research-implementation-plan.md)
> 原则延续 M1:领域模型唯一事实源、全量渲染管道零特殊分支、raw_extra/global_extra 兜底不可识别内容、单写者下发。

## 1. 需求与任务拆解(顺序即执行顺序)

| ID | 需求 | 要点 | 插件依赖 |
| --- | --- | --- | --- |
| T2.1 | FR-701 上游健康可视化 | `GET /reverse_proxy/upstreams` 透传展示(地址/请求/失败数);站点表单可配主动健康检查(路径/间隔/预期码)渲染进 reverse_proxy 块 | 无(主动健康检查为 Caddy 内置) |
| T2.2 | FR-704 访问控制 | Basic Auth 用户管理(panel 做 bcrypt);IP 白/黑名单(CIDR);渲染 basicauth 与 remote_ip matcher。限流为插件 —— 仅表单预留字段并标注依赖,M2 不实现渲染 | 限流:xcaddy(标注) |
| T2.3 | FR-705 导入 Caddyfile | 预览(解析→可识别映射+剩余片段)→ 确认落库;导入不自动下发(默认落库为停用,由用户逐个启用) | 无 |
| T2.4 | FR-706 备份与恢复 | 一键导出 JSON(sites/config_versions/settings 可编辑项);导入校验后替换落库并提示重新下发 | 无 |
| T2.5 | FR-702 监控图表 | panel 定时抓取 Admin API `/metrics`(metrics 全局开关已预留)存 SQLite,内置折线图;一键生成 Prometheus 抓取配置 | 无 |
| T2.6 | FR-703 证书管理 | 手动上传证书(渲染 `tls` 指令);ACME 账号邮箱(settings→全局块 `email`);到期检查与通知(Webhook/SMTP);DNS challenge 表单占位(渲染 `{env.*}` 占位符,不落明文凭据) | DNS challenge:xcaddy(标注) |

## 2. 数据模型变更(AutoMigrate 自动加列,无需迁移脚本)

```
sites 增列
  health_check_enabled  BOOL            -- FR-701 主动健康检查开关
  health_check_path     TEXT            -- 如 /healthz
  health_check_interval TEXT            -- Go duration,如 10s
  health_check_expected INTEGER         -- 预期状态码,默认 200
  basic_auth_users      TEXT            -- JSON [{"username","password_hash"}](bcrypt,FR-704)
  ip_allowlist          TEXT            -- 逗号分隔 CIDR(FR-704)
  ip_blocklist          TEXT            -- 逗号分隔 CIDR
  cert_file             TEXT            -- FR-703 手动上传证书路径(共享卷)
  key_file              TEXT            -- 私钥路径
  rate_limit            TEXT            -- FR-704 限流表单占位(M2 不渲染,仅存储)

新表 metrics_samples                    -- FR-702 抓取样本
  id, name(TEXT), labels(TEXT JSON), value(REAL), ts(TIMESTAMP)
  索引 (name, ts);保留 24h,后台定期清理

settings 新键
  acme_email          -- FR-703 渲染全局块 email 指令
  notify_webhook_url  -- FR-703 到期提醒 Webhook
  notify_smtp_*       -- host/port/username/password/from/to(密码值存 settings,不回显)
```

## 3. 渲染变更(模板)

- `site_proxy.tmpl`:健康检查行(`health_uri / health_interval / health_status`);`basicauth` 指令(bcrypt hash 原样);IP 名单(`@blocked remote_ip ...` + `abort @blocked`;白名单语义为"名单外 abort")。
- `site.tmpl` 层(三类站点共有):`tls <cert> <key>`(手动上传时);IP 名单 matcher 对 static/redirect 同样生效 → 抽到公共站点体。
- `global.tmpl`:`email <acme_email>`(设置后渲染)。
- 所有新增渲染均须同步 golden 测试与"产物必过离线校验"契约测试。

## 4. API 增量(REST,/api/v1)

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/upstreams` | 透传 `GET /reverse_proxy/upstreams`(FR-701) |
| POST | `/import/preview`、`/import/apply` | 导入预览/确认(FR-705) |
| GET | `/backup`、POST `/restore` | 备份导出/导入(FR-706) |
| GET | `/metrics/summary?window=` | 抓取样本聚合(FR-702) |
| GET | `/metrics/prometheus-config` | 生成 Prometheus 抓取配置 YAML(FR-702) |
| POST | `/sites/{id}/certificate/upload` | 手动上传证书/私钥(FR-703,写入共享卷) |

设置页扩展:`acme_email`、通知渠道键(白名单与校验随 setting service 登记)。

## 5. 前端增量

- Sites 页表单:健康检查、Basic Auth(kv + 密码生成)、IP 名单区块;证书 Tab 展示上传入口。
- 新页面:上游状态(`/upstreams`)、导入向导(预览 diff)、监控(`/metrics`,图表库选型 **recharts** —— React 生态事实标准,轻量;echarts 备选)。
- Settings 页:ACME 邮箱、通知渠道、Prometheus 配置一键复制/下载。

## 6. 风险与对策

| 风险 | 对策 |
| --- | --- |
| 导入解析遗漏(复杂路由/片段嵌套) | 只做"逐指令白名单识别",其余整段落 raw_extra;预览页强提示"导入后请核对" |
| 手动上传证书路径逃逸 | 服务端固定写入共享卷 `/data/certs/<siteID>/`,文件名服务端生成,不接受用户路径 |
| metrics 样本膨胀 | 单指标 24h 保留 + 抓取间隔 15s 上限约 5.8k 行/指标,后台每日清理 |
| basicauth hash 破坏 Caddyfile 语法 | hash 为 `$2a$...` 含 `$`,Caddy 原生支持;渲染时不加引号原样输出 |
| 插件依赖(限流/DNS challenge) | 面板仅产出配置并显式标注"需要带插件的 Caddy";后续提供 xcaddy Dockerfile 变体(T3 可选) |

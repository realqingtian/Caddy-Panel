# Caddy Panel

**给 Caddy 一个好用的 Web 管理面板。** 不用再手写 Caddyfile:在网页上填表单建站点,保存即生效;改错了能一键回滚;多台服务器在一个面板里统一管理。

- 🧩 **表单化配置**:反向代理 / 静态站点 / 重定向 / TCP·UDP 透传,健康检查、负载均衡、WebSocket 开关都是表单项
- 🚀 **保存即生效**:面板自动生成 Caddyfile → 校验 → 热加载,配置错误不会让线上服务挂掉,失败原因(Caddy 错误原文)直接展示
- ⏪ **版本与回滚**:每次下发自动留版本快照,任何一次成功配置都能一键回滚
- 🖥️ **多节点管理**:在其他服务器装一个轻量 Agent,同一面板管理多台机器的 Caddy(反向连接,被管机无需开放管理端口)
- 🔐 **安全与访问控制**:Basic Auth、IP 黑白名单、按 IP 限流、HTTPS 证书状态与到期提醒、开放 API Token + 只读角色
- 📊 **可观测**:请求量/流量监控图表、访问与错误日志在线查看、配置漂移检测
- 🌏 中/英双语、亮/暗主题,单二进制 + Docker Compose 一键部署

> 上游 [Caddy](https://caddyserver.com) 承载流量,面板只负责"管配置":所有配置变更都经过校验后热加载并落盘,Caddy 重启后自动恢复。面板不代理业务流量,性能零损耗。

## 快速开始

只需 Docker:

```bash
git clone https://github.com/realqingtian/Caddy-Panel.git
cd Caddy-Panel/deploy

# 建议预置管理员密码(不设置则首次启动随机生成,打印在面板启动日志里,只出现一次)
export PANEL_ADMIN_PASSWORD=your-password

docker compose up -d   # 首次启动需本地构建镜像(含带插件的 Caddy),约几分钟
```

然后:

1. 打开 `http://localhost:8080`,用账号 `admin` 和你设置的密码登录
2. 左侧「站点管理」→「新建站点」,选类型填信息(例如反向代理:域名 `app.example.com`,上游 `127.0.0.1:8080`),保存
3. 完成 —— 配置已热加载生效,并同时落盘(Caddy 重启不丢)

> 部署到公网时,把域名解析到服务器并开放 80/443,Caddy 会自动签发 HTTPS 证书。

### 下一步

| 想做什么 | 去哪里 |
| --- | --- |
| 限制谁能访问站点(Basic Auth / IP 名单 / 限流) | 站点编辑表单内的「访问控制」区块 |
| 查看/回滚历史配置、直接编辑 Caddyfile | 「配置中心」 |
| 看请求量趋势、在线查日志 | 「监控」/「日志」(监控需在「设置」页开启 metrics) |
| 上传自己的证书、配置 DNS 验证签发、到期提醒 | 站点「证书」操作 /「设置」页 |
| 备份全部配置(下载 JSON)/ 恢复 | 「设置」页备份卡片 |
| 在别的服务器上管理 Caddy | 见下方[多节点管理](#多节点管理) |
| 给自动化脚本开 API | 「设置」页 API Token(支持只读/读写权限) |

## 功能总览

| 模块 | 你能做什么 |
| --- | --- |
| 站点管理 | 反向代理(多上游、负载均衡策略、WebSocket、自定义响应头、主动健康检查)、静态站点、重定向;启用/停用即时生效;支持 Caddyfile 导入(把现有配置搬进面板) |
| l4 透传 | TCP/UDP 端口透传(如数据库、DNS 转发),与 HTTP 站点并存(业务端口需自行在 compose/防火墙放行) |
| 访问控制 | Basic Auth、IP 黑白名单、按客户端 IP 限流(限流/DNS 验证需要自带插件的 Caddy 镜像,compose 已内置) |
| 证书 | 站点证书签发状态查看、手动上传证书、ACME 邮箱设置、DNS 验证(Cloudflare 等)、到期前自动提醒(Webhook / SMTP) |
| 配置中心 | 当前配置预览、Caddyfile 直接编辑(强制校验,防止改坏)、版本历史、一键回滚 |
| 监控与日志 | 请求量/耗时/流量图表(按域名分线)、访问与错误日志在线查看、配置漂移检测(有人手改了配置会告警) |
| 多节点 | 一个面板管理多台服务器的 Caddy,站点按节点隔离,按节点下发与回滚 |
| 开放能力 | API Token(只读/读写)、OpenAPI 文档(`/swagger/index.html`)、Web 终端(管理员) |
| 界面 | 中/英双语、亮/暗主题、后端错误消息同步本地化 |

## 多节点管理

一台服务器跑面板(中心),其他服务器各装一个 Agent,即可在同一个界面管理所有机器:

- Agent **主动连出**到面板(WebSocket),被管机不用开放任何管理端口
- 每个节点的站点独立渲染、独立下发;节点离线时该节点操作会明确报错,不会静默丢失
- 接入只需三步:面板「节点管理」→ 填节点名生成安装命令 → 到目标机粘贴执行

```bash
# 安装命令由面板生成(下载对应平台二进制 → 写入 systemd 服务 → 内嵌一次性接入令牌)。
# 若对应平台的 Release 二进制尚未发布,可先从源码构建并自行分发:
cd agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o caddy-panel-agent .
```

前提与约定:

- 目标机为 Linux amd64/arm64 且使用 systemd;面板「设置」页需配置对外可达地址(用于生成安装命令)
- 接入令牌一次性(24 小时过期、可撤销);Agent 心跳 30s,90 秒无心跳判定离线
- Agent 也可从源码构建:

```bash
cd agent
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o caddy-panel-agent .
```

## 配置参考

面板通过 `PANEL_*` 环境变量配置;除 JWT 相关系外,均可事后在「设置」页修改(env 只作首次默认值)。

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `PANEL_LISTEN` | `:8080` | 面板监听地址 |
| `PANEL_DB_PATH` | `./data/panel.db` | SQLite 路径 |
| `PANEL_ADMIN_URL` | `http://localhost:2019` | 面板连接 Caddy Admin API 的地址(`http(s)://` 或 `unix://`) |
| `PANEL_CADDY_ADMIN_BIND` | `localhost:2019` | 渲染到 Caddyfile 全局块的 admin 监听地址,**必须与面板实际可达地址一致**(见故障排查) |
| `PANEL_CADDYFILE_PATH` | `./data/Caddyfile` | 渲染产物落盘路径 |
| `PANEL_ADMIN_PASSWORD` | 随机(仅首次) | 首次启动的管理员密码 |
| `PANEL_JWT_SECRET` / `PANEL_JWT_TTL` | 随机(持久化) / `24h` | 登录会话签名与有效期(仅 env 可改) |
| `PANEL_LOGIN_MAX_FAILS` / `PANEL_LOGIN_LOCK_WINDOW` | `5` / `1m` | 登录失败锁定(设置页可调) |
| `PANEL_LOG_ACCESS_PATH` / `PANEL_LOG_ERROR_PATH` | `./data/*.log` | 日志在线查看的读取目标 |
| `PANEL_CERT_STORAGE_PATH` | `/data/caddy` | Caddy 证书存储(只读,用于证书状态展示) |
| `PANEL_PUBLIC_URL` | 空 | 面板对外地址(生成多节点安装命令用) |

## 故障排查

**保存后网站 502 / 未生效?**
面板保存即热加载,失败时不会应用新配置。看保存时的报错通知(Caddy 错误原文会原样展示)与「配置中心」的失败版本记录。上游地址写错(如容器内连不到宿主机)是最常见原因。

**仪表盘提示配置漂移?**
说明有人绕过面板改了 Caddy 配置或直接改了落盘的 Caddyfile。在「配置中心」查看差异后,可从任意成功版本回滚恢复一致。

**面板突然连不上 Caddy(所有操作报 Caddy 不可达)?**
典型原因是 Caddyfile 全局块的 `admin` 监听地址被改成与面板实际可达地址不一致(如直编时删掉或改错)。修复:直接编辑 Caddyfile 恢复 `admin` 行,或重启 Caddy 以落盘配置恢复。配置中心对"删除 admin 指令"的直编有防护,但手动改文件绕不过。

**远程节点显示离线?**
节点管理页看「最近心跳」。通常是目标机 Agent 进程未运行(`systemctl status caddy-panel-agent`)或到面板的网络不通。节点离线期间对其的操作会直接报错,恢复在线后重试即可。

**限流 / DNS 验证 / l4 报 unknown directive?**
这些能力需要带插件的 Caddy。compose 默认构建的 `caddy` 服务已内置(限流 + Cloudflare DNS 验证 + l4);若你换成官方 `caddy:2` 镜像,对应功能不可用。

## 从源码构建 / 开发

依赖:Go 1.27+、Bun 1.2+(可选:本机 Caddy 用于真实下发联调)。

```bash
# 方式一:源码跑后端(:8080,内嵌已构建前端)+ Compose 提供 Caddy
cd backend && go run ./cmd/panel

# 方式二:前后端分离开发(前端 :5173 热更新,/api 代理到 :8080)
cd frontend && bun install && bun run dev
```

```bash
# 常用工程命令
cd backend && go test ./...                    # 后端单测
cd frontend && bun run lint && bun run build   # 前端 lint + 构建(产物嵌入后端)
```

## 项目文档

| 文档 | 内容 |
| --- | --- |
| [需求规格 PRD](docs/requirements.md) | 功能需求(编号化)与验收标准、非功能需求 |
| [架构与目录设计](docs/project-structure.md) | 运行时架构、目录结构、关键数据流转、环境变量约定 |
| [技术栈选型](docs/tech-stack.md) | 各层选型与版本、选型理由 |
| [M1 设计](docs/m1-design.md) / [M2 设计](docs/m2-design.md) / [M3 设计](docs/m3-design.md) | 各里程碑详细设计(数据模型、渲染管道、多节点协议等) |
| [实现方案调研](docs/research-implementation-plan.md) | Caddy 管理能力调研、竞品对比、架构方案论证 |
| [进度与决策日志](docs/progress.md) | 里程碑状态、任务看板、全部设计决策及理由 |
| [AGENTS.md](AGENTS.md) | AI 代理/贡献者协作须知(查阅 antd 6 文档姿势、工程约定、提交检查) |

## 安全注意事项

- **Caddy Admin API 不要暴露公网**:它是无鉴权的管理入口。compose 中仅容器网络可达;生产建议 panel 与 Caddy 共享网络命名空间并改用 `unix://` 连接
- 面板为单实例设计(进程内单写者保证配置一致性),请固定单副本部署
- Web 终端以 panel 进程用户执行 shell(容器内建议以非 root 运行);API Token 只存哈希,创建时请立即保存
- 建议把面板置于反向代理/VPN 之后,不要将 8080 直接暴露公网

## License

[Apache-2.0](LICENSE)

# 通知渠道适配设计(Webhook 多渠道)— 迭代 I6 后半段

> 状态:待评审。评审通过后开工;完成后按惯例整篇归档至 docs/archive/。
> 背景:FR-703 证书到期提醒的 Webhook 渠道目前发送**自定格式原始 JSON**,只有自建接收端能解析;用户配置飞书机器人后收到的是无效消息(实测)。本设计适配主流 IM 的群机器人 Webhook。

## 1. 渠道与消息格式

统一先构造**纯文本摘要**(一行事件描述,如 `证书即将到期:caddy-panel.realqingtian.com 将于 2026-11-20 到期`),再按渠道包装:

| 渠道 | URL 识别特征(auto 模式) | Payload |
| --- | --- | --- |
| 飞书 | `open.feishu.cn/open-apis/bot` | `{"msg_type":"text","content":{"text":text}}` |
| 钉钉 | `oapi.dingtalk.com/robot` | `{"msgtype":"text","text":{"content":text}}` |
| 企业微信 | `qyapi.weixin.qq.com/cgi-bin/webhook` | `{"msgtype":"text","text":{"content":text}}` |
| Slack | `hooks.slack.com` | `{"text":text}` |
| Discord | `discord.com/api/webhooks` | `{"content":text}` |
| 通用 JSON | 其余 | 现有原始 JSON(向后兼容,不破坏已部署的自建接收端) |

**决策**:v1 消息一律纯文本(各平台卡片/markdown 语法差异大且经常要求加签/关键词,纯文本通过率最高);渠道加签(钉钉 secret/飞书 signature)列为后续增强,不在本期。

## 2. 配置与交互

- settings 新键 `notify_webhook_type`:枚举 `auto / feishu / dingtalk / wecom / slack / discord / json`,默认 `auto`(按 URL 特征识别,识别不出按通用 JSON);
- 进入设置白名单(editableSettingKeys)+ 枚举校验(settingValidators),运行时生效;
- 设置页"证书到期提醒 Webhook"输入框旁新增**通知类型 Select**(自动识别 + 五渠道 + 通用 JSON)与**"发送测试消息"按钮**(按当前配置向 Webhook 发一条测试消息,与 SMTP 测试邮件对称);
- 双渠道并存语义不变:SMTP 与 Webhook 任一配置即发送,互不影响。

## 3. 代码改动面

| 层 | 改动 |
| --- | --- |
| backend service | 新建 `notify.go`:渠道识别 + payload 构造器(各渠道构造函数 + golden 单测);`cert_expiry.go` 的 notifyWebhook 改走构造器 |
| backend settings | 白名单/校验加 `notify_webhook_type` |
| backend handler | `notify.go` 增加 WebhookTest(读配置 → 构造 → POST → 失败 502 带原因) |
| frontend | Settings 通知区:类型 Select + 测试按钮;i18n 双语 |
| 测试 | payload 构造 golden 测试(六渠道);handler 未配置 400/发送失败 502 各一条 |

## 4. 验收标准

1. 用户真实飞书机器人收到格式化文本消息(主验收);
2. 钉钉/企微/Slack/Discord 的 payload 由 golden 测试锁定格式(用户有条件时自测);
3. 通用 JSON 模式与 I6 之前行为逐字节一致(向后兼容);
4. auto 模式对五个官方域名的 URL 识别全部正确;
5. "发送测试消息"对未配置 Webhook 返回 400、发送失败返回 502 且 detail 含服务端返回体。

## 5. Webhook 可视化配置与测试连通性

- 设置页通知分类内可视化配置:Webhook URL 输入框 + 通知类型 Select(auto/五渠道/通用 JSON),类型选择"自动识别"时选择器下方实时显示识别结果(如"已识别:飞书机器人");
- **测试连通性**:按当前 URL+类型真实 POST 一条测试消息——HTTP 2xx 判定成功(提示"测试消息已发送"),非 2xx/网络失败给出失败原因(detail 保留服务端返回体);URL 为空时按钮禁用;
- 测试消息内容:`[Caddy Panel] Webhook 渠道测试消息:收到即说明渠道配置成功`(各渠道按 §1 格式包装)。

## 6. 非目标

- 消息卡片/markdown 模板、渠道加签、钉钉加签关键词、发送重试队列、更多渠道(Telegram 需 bot token+chat_id 单独建模,有需求再立);
- 通知事件扩展(本期仅证书到期提醒这一事件源)。

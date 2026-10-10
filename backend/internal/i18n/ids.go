// Package i18n — 消息 ID 常量与各语言兜底文案。
// 新增消息:在此登记 ID,并同步补 locales/zh-CN.json 与 en-US.json 两份文案。
package i18n

// 消息 ID;fallback 文案取中文(默认语言),未命中语言包时兜底。
const (
	MsgInvalidCredentials   = "error.invalid_credentials"
	MsgAccountLocked        = "error.account_locked"
	MsgUnauthorized         = "error.unauthorized"
	MsgBadRequest           = "error.bad_request"
	MsgNotFound             = "error.not_found"
	MsgInternal             = "error.internal"
	MsgForbidden            = "error.forbidden"
	MsgSiteNotFound         = "error.site_not_found"
	MsgSiteDomainConflict   = "error.site_domain_conflict"
	MsgSiteDeployFailed     = "error.site_deploy_failed"
	MsgConfigDeployFailed   = "error.config_deploy_failed"
	MsgVersionNotApplicable = "error.version_not_applicable"
	MsgCaddyUnreachable     = "error.caddy_unreachable"
	MsgNodeNotFound         = "error.node_not_found"
	MsgNodeHasSites         = "error.node_has_sites"
)

// FallbackText 各消息的中文兜底文案;英文文案只存在于语言包中。
var FallbackText = map[string]string{
	MsgInvalidCredentials:   "用户名或密码错误",
	MsgAccountLocked:        "登录失败次数过多,已临时锁定,请稍后再试",
	MsgUnauthorized:         "未登录或会话已过期,请重新登录",
	MsgBadRequest:           "请求参数不正确",
	MsgNotFound:             "接口不存在",
	MsgInternal:             "服务器内部错误,请稍后再试",
	MsgForbidden:            "没有权限执行该操作",
	MsgSiteNotFound:         "站点不存在",
	MsgSiteDomainConflict:   "域名已被其他站点占用",
	MsgSiteDeployFailed:     "配置校验或下发失败,未做任何变更",
	MsgConfigDeployFailed:   "配置校验或下发失败,未做任何变更;原因见 detail",
	MsgVersionNotApplicable: "只能回滚到下发成功的版本",
	MsgCaddyUnreachable:     "Caddy Admin API 不可达,请检查 Caddy 是否运行",
	MsgNodeNotFound:         "节点不存在",
	MsgNodeHasSites:         "节点名下仍有站点,请先删除站点",
}

// Package service — 系统设置:KV 读写(env 提供默认,settings 表覆盖,FR-601)。
package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"caddy-panel/backend/internal/model"
)

// SettingRepo setting service 对数据层的依赖。
type SettingRepo interface {
	GetSetting(ctx context.Context, key string) (string, bool, error)
	SetSetting(ctx context.Context, key, value string) error
}

// editableSettingKeys 设置页可读写的键(FR-601);JWT 密钥/有效期不在其列。
var editableSettingKeys = []string{
	model.SettingKeyAdminURL,
	model.SettingKeyCaddyAdminBind,
	model.SettingKeyCaddyfilePath,
	model.SettingKeyLogAccessPath,
	model.SettingKeyLogErrorPath,
	model.SettingKeyLoginMaxFails,
	model.SettingKeyLoginLockWindow,
	model.SettingKeyMetricsEnabled,
	model.SettingKeyGlobalExtra,
}

// editableKeySet 白名单集合(Update 拒绝白名单外键)。
var editableKeySet = func() map[string]bool {
	set := make(map[string]bool, len(editableSettingKeys))
	for _, k := range editableSettingKeys {
		set[k] = true
	}
	return set
}()

// settingValidators 需要值校验的键及规则;未登记的键不做格式校验。
var settingValidators = map[string]func(string) error{
	model.SettingKeyLoginMaxFails: func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return fmt.Errorf("登录锁定阈值必须为正整数")
		}
		return nil
	},
	model.SettingKeyLoginLockWindow: func(v string) error {
		if _, err := time.ParseDuration(v); err != nil {
			return fmt.Errorf("登录锁定时长必须为 Go duration 格式,如 1m、30s")
		}
		return nil
	},
	model.SettingKeyMetricsEnabled: func(v string) error {
		if v != "true" && v != "false" {
			return fmt.Errorf("metrics 开关必须为 true 或 false")
		}
		return nil
	},
}

// SettingService 系统设置业务;锁定参数每次读取实现"立即生效",
// 路径/地址类在下次操作时生效 —— 读写双方都已按此实现。
type SettingService struct {
	repo     SettingRepo
	defaults map[string]string // env 提供的默认值
}

// NewSetting 构造设置服务;defaults 为 env 默认值(键为 settings key)。
func NewSetting(repo SettingRepo, defaults map[string]string) *SettingService {
	return &SettingService{repo: repo, defaults: defaults}
}

// SettingsView 设置读取结果(key → 生效值)。
type SettingsView map[string]string

// All 返回全部可编辑设置项的生效值(settings 表优先,env 默认兜底)。
func (s *SettingService) All(ctx context.Context) (SettingsView, error) {
	view := SettingsView{}
	for _, key := range editableSettingKeys {
		view[key] = s.defaults[key]
		v, ok, err := s.repo.GetSetting(ctx, key)
		if err != nil {
			return nil, err
		}
		if ok && v != "" {
			view[key] = v
		}
	}
	return view, nil
}

// Update 校验并持久化一批设置;键不在白名单即拒绝,全部校验通过后才写入。
func (s *SettingService) Update(ctx context.Context, values map[string]string) error {
	for key, value := range values {
		if !editableKeySet[key] {
			return fmt.Errorf("未知设置项: %s", key)
		}
		if validate := settingValidators[key]; validate != nil {
			if err := validate(value); err != nil {
				return &InvalidInputError{Reason: err.Error()}
			}
		}
	}
	for key, value := range values {
		if err := s.repo.SetSetting(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

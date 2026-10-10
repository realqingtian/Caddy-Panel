package service

import (
	"context"

	"caddy-panel/backend/internal/caddy"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/store"
)

// localApplierFor 测试用 ApplierFor:全部节点路由到 local 实现(路径不落盘校验的场景)。
func localApplierFor(client caddy.Client) caddy.ApplierFor {
	return localApplierForWith(client, func() string { return "/tmp/unused-Caddyfile" })
}

// localApplierForWith 同上,但落盘路径由用例指定。
func localApplierForWith(client caddy.Client, pathFn func() string) caddy.ApplierFor {
	return func(string) (caddy.Applier, error) {
		return caddy.NewLocalApplier(client, pathFn), nil
	}
}

// fakeNodeGetter 节点查询 fake:一律返回不存在(local 站点不会触发查询)。
type fakeNodeGetter struct{}

func (fakeNodeGetter) GetNodeByNodeID(context.Context, string) (*model.Node, error) {
	return nil, store.ErrNotFound
}

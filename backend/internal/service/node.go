// Package service — 节点管理(T3.3):接入令牌签发、安装命令生成、
// 节点生命周期(注册/心跳/删除防护/离线判定)。
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"caddy-panel/backend/internal/agentproto"
	"caddy-panel/backend/internal/model"
)

// 凭证前缀:与 API Token(cpt_)并列的两类节点凭证。
const (
	EnrollTokenPrefix = "cen_" // 一次性注册令牌
	NodeSecretPrefix  = "cnd_" // 注册成功后签发的节点密钥
)

// 平台矩阵(m3-design §3.1):安装命令按平台下载对应二进制。
var agentPlatforms = map[string]bool{
	"linux-amd64": true,
	"linux-arm64": true,
}

// 心跳节奏:agent 30s 一帧,中心 3 倍间隔(90s)无心跳判 offline;
// 扫描循环 15s 一轮。
const (
	HeartbeatInterval   = 30 * time.Second
	NodeOfflineAfter    = 3 * HeartbeatInterval
	StaleScanInterval   = 15 * time.Second
	enrollTokenLifetime = 24 * time.Hour
)

// 业务错误。
var (
	ErrNodeNotFound       = errors.New("node not found")
	ErrNodeHasSites       = errors.New("node still has sites")
	ErrEnrollTokenInvalid = errors.New("enrollment token invalid")
	ErrNodeNameTaken      = errors.New("node name already taken")
)

// NodeRepo node service 对数据层的依赖;由 store 实现。
type NodeRepo interface {
	CreateNode(ctx context.Context, node *model.Node) error
	GetNodeByNodeID(ctx context.Context, nodeID string) (*model.Node, error)
	GetNodeByName(ctx context.Context, name string) (*model.Node, error)
	GetNodeBySecretHash(ctx context.Context, hash string) (*model.Node, error)
	ListNodes(ctx context.Context) ([]model.Node, error)
	UpdateNode(ctx context.Context, node *model.Node) error
	DeleteNode(ctx context.Context, id uint) error
	MarkStaleNodesOffline(ctx context.Context, olderThan time.Duration) (int64, error)
	CountSitesByNode(ctx context.Context, nodeID string) (int64, error)
	CreateEnrollmentToken(ctx context.Context, token *model.EnrollmentToken) error
	GetEnrollmentTokenByHash(ctx context.Context, hash string) (*model.EnrollmentToken, error)
	ListEnrollmentTokens(ctx context.Context) ([]model.EnrollmentToken, error)
	UpdateEnrollmentToken(ctx context.Context, token *model.EnrollmentToken) error
	DeleteEnrollmentToken(ctx context.Context, id uint) error
	GetSetting(ctx context.Context, key string) (string, bool, error)
}

// NodeHub node service 对连接注册表的依赖。
type NodeHub interface {
	IsOnline(nodeID string) bool
	DropNode(nodeID string)
}

// NodeDeps node service 依赖。
type NodeDeps struct {
	Repo      NodeRepo
	Hub       NodeHub
	PanelVer  string // 中心版本(握手应答携带,供 agent 兼容性观测)
}

// Node 节点业务服务。
type Node struct {
	repo       NodeRepo
	hub        NodeHub
	panelVer   string
}

// NewNode 构造节点服务。
func NewNode(d NodeDeps) *Node {
	return &Node{repo: d.Repo, hub: d.Hub, panelVer: d.PanelVer}
}

// EnrollOutcome 注册/续连结果;PlainSecret 仅新注册时非空(一次性下发)。
type EnrollOutcome struct {
	Node        *model.Node
	PlainSecret string
	Enrolled    bool // true=新注册,false=已有密钥续连
}

// Enroll 处理握手凭证:cen_* 走一次性注册(创建节点+签发密钥),
// cnd_* 走密钥续连。失败一律拒绝(协议版本检查在 WS handler)。
func (s *Node) Enroll(ctx context.Context, hello agentproto.ClientHello) (*EnrollOutcome, error) {
	switch {
	case strings.HasPrefix(hello.Credential, EnrollTokenPrefix):
		return s.enrollNew(ctx, hello)
	case strings.HasPrefix(hello.Credential, NodeSecretPrefix):
		return s.resumeWithSecret(ctx, hello)
	default:
		return nil, ErrEnrollTokenInvalid
	}
}

// enrollNew 一次性注册:令牌校验(未用/未过期/未撤销)→ 节点名防重 →
// 创建节点并签发密钥 → 令牌作废。
func (s *Node) enrollNew(ctx context.Context, hello agentproto.ClientHello) (*EnrollOutcome, error) {
	token, err := s.loadValidToken(ctx, hello.Credential)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.GetNodeByName(ctx, token.NodeName); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrNodeNameTaken, token.NodeName)
	}

	secret, err := randomCredential(NodeSecretPrefix, 24)
	if err != nil {
		return nil, err
	}
	nodeID, err := randomNodeID()
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(secret))
	node := &model.Node{
		NodeID:       nodeID,
		Name:         token.NodeName,
		SecretHash:   hex.EncodeToString(sum[:]),
		SecretPre:    secret[:len(NodeSecretPrefix)+8],
		Status:       model.NodeStatusOnline,
		AgentVersion: hello.AgentVersion,
		Platform:     hello.Platform,
		AdminBind:    hello.LocalConfig.AdminBind,
		CaddyfilePath: hello.LocalConfig.CaddyfilePath,
		ErrorLogPath:  hello.LocalConfig.ErrorLogPath,
		AccessLogPath: hello.LocalConfig.AccessLogPath,
		CaddyVersion:  hello.LocalConfig.CaddyVersion,
		EnrolledAt:    time.Now(),
	}
	now := time.Now()
	node.LastSeenAt = &now

	if err := s.repo.CreateNode(ctx, node); err != nil {
		return nil, err
	}
	usedAt := now
	token.UsedAt = &usedAt
	if err := s.repo.UpdateEnrollmentToken(ctx, token); err != nil {
		// 注册已完成,令牌作废失败只留日志(下次注册会因节点重名被拒,可人工清理)。
		slog.Error("作废接入令牌失败", "token_id", token.ID, "error", err)
	}
	slog.Info("新节点注册成功", "node", node.Name, "node_id", node.NodeID, "platform", node.Platform)
	return &EnrollOutcome{Node: node, PlainSecret: secret, Enrolled: true}, nil
}

// resumeWithSecret 节点密钥续连:哈希查找即认证(密钥等同身份)。
func (s *Node) resumeWithSecret(ctx context.Context, hello agentproto.ClientHello) (*EnrollOutcome, error) {
	sum := sha256.Sum256([]byte(hello.Credential))
	node, err := s.repo.GetNodeBySecretHash(ctx, hex.EncodeToString(sum[:]))
	if err != nil {
		return nil, ErrEnrollTokenInvalid
	}
	return &EnrollOutcome{Node: node, Enrolled: false}, nil
}

// loadValidToken 校验注册令牌状态。
func (s *Node) loadValidToken(ctx context.Context, plain string) (*model.EnrollmentToken, error) {
	sum := sha256.Sum256([]byte(plain))
	token, err := s.repo.GetEnrollmentTokenByHash(ctx, hex.EncodeToString(sum[:]))
	if err != nil {
		return nil, ErrEnrollTokenInvalid
	}
	if token.Revoked || token.UsedAt != nil {
		return nil, fmt.Errorf("%w: 令牌已使用或已撤销", ErrEnrollTokenInvalid)
	}
	if time.Now().After(token.ExpiresAt) {
		return nil, fmt.Errorf("%w: 令牌已过期", ErrEnrollTokenInvalid)
	}
	return token, nil
}

// CreateEnrollmentTokenResult 签发结果:明文与安装命令仅此一次。
type CreateEnrollmentTokenResult struct {
	PlainToken     string
	Token          *model.EnrollmentToken
	InstallCommand string
}

// CreateEnrollmentToken 签发注册令牌并生成安装命令(节点名/平台随令牌预置)。
func (s *Node) CreateEnrollmentToken(ctx context.Context, nodeName, platform string) (*CreateEnrollmentTokenResult, error) {
	nodeName = strings.TrimSpace(nodeName)
	if nodeName == "" {
		return nil, &InvalidInputError{Reason: "节点名不能为空"}
	}
	if len(nodeName) > 128 {
		return nil, &InvalidInputError{Reason: "节点名过长(≤128)"}
	}
	if !agentPlatforms[platform] {
		return nil, &InvalidInputError{Reason: fmt.Sprintf("平台不支持(可选: linux-amd64, linux-arm64): %s", platform)}
	}

	plain, err := randomCredential(EnrollTokenPrefix, 24)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(plain))
	token := &model.EnrollmentToken{
		NodeName:  nodeName,
		Platform:  platform,
		Prefix:    plain[:len(EnrollTokenPrefix)+8],
		TokenHash: hex.EncodeToString(sum[:]),
		ExpiresAt: time.Now().Add(enrollTokenLifetime),
	}
	if err := s.repo.CreateEnrollmentToken(ctx, token); err != nil {
		return nil, err
	}
	centerURL := s.settingOr(ctx, model.SettingKeyPanelPublicURL, "")
	return &CreateEnrollmentTokenResult{
		PlainToken:     plain,
		Token:          token,
		InstallCommand: InstallCommand(centerURL, plain, platform),
	}, nil
}

// ListEnrollmentTokens 令牌列表。
func (s *Node) ListEnrollmentTokens(ctx context.Context) ([]model.EnrollmentToken, error) {
	return s.repo.ListEnrollmentTokens(ctx)
}

// RevokeEnrollmentToken 撤销令牌(尚未使用时防止被截获利用)。
func (s *Node) RevokeEnrollmentToken(ctx context.Context, id uint) error {
	tokens, err := s.repo.ListEnrollmentTokens(ctx)
	if err != nil {
		return err
	}
	for i := range tokens {
		if tokens[i].ID == id {
			tokens[i].Revoked = true
			return s.repo.UpdateEnrollmentToken(ctx, &tokens[i])
		}
	}
	return nil
}

// DeleteEnrollmentToken 删除令牌记录。
func (s *Node) DeleteEnrollmentToken(ctx context.Context, id uint) error {
	return s.repo.DeleteEnrollmentToken(ctx, id)
}

// ListNodes 节点列表;连接在线的节点即时呈现 online(不等心跳周期)。
func (s *Node) ListNodes(ctx context.Context) ([]model.Node, error) {
	nodes, err := s.repo.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	for i := range nodes {
		if s.hub.IsOnline(nodes[i].NodeID) {
			nodes[i].Status = model.NodeStatusOnline
		}
	}
	return nodes, nil
}

// DeleteNode 删除节点:有站点先删站点(m3-design §3.3 第 6 条);
// 删除即断连吊销密钥(agent 下次连接被拒,需重新 enrollment)。
func (s *Node) DeleteNode(ctx context.Context, nodeID string) error {
	if nodeID == model.NodeIDLocal {
		return &InvalidInputError{Reason: "local 节点为面板内置,不可删除"}
	}
	node, err := s.repo.GetNodeByNodeID(ctx, nodeID)
	if err != nil {
		return ErrNodeNotFound
	}
	count, err := s.repo.CountSitesByNode(ctx, nodeID)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: 节点 %s 名下还有 %d 个站点,请先删除", ErrNodeHasSites, node.Name, count)
	}
	if err := s.repo.DeleteNode(ctx, node.ID); err != nil {
		return err
	}
	s.hub.DropNode(nodeID)
	slog.Info("节点已删除(密钥吊销)", "node", node.Name, "node_id", nodeID)
	return nil
}

// WatchStale 后台循环:周期把超过 90s 无心跳的节点置 offline(90s 判定,
// m3-design §3.3 第 3 条);随 graceful shutdown 收敛。
func (s *Node) WatchStale(ctx context.Context) {
	ticker := time.NewTicker(StaleScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			affected, err := s.repo.MarkStaleNodesOffline(ctx, NodeOfflineAfter)
			if err != nil {
				slog.Error("刷新离线节点失败", "error", err)
				continue
			}
			if affected > 0 {
				slog.Warn("有节点超过心跳阈值,已置为 offline", "count", affected)
			}
		}
	}
}

// settingOr settings 读取(env 默认由白名单注册兜底,这里仅取表值)。
func (s *Node) settingOr(ctx context.Context, key, def string) string {
	v, ok, err := s.repo.GetSetting(ctx, key)
	if err != nil || !ok || v == "" {
		return def
	}
	return v
}

// randomCredential 生成带前缀的随机凭证明文。
func randomCredential(prefix string, randomBytes int) (string, error) {
	buf := make([]byte, randomBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成随机凭证: %w", err)
	}
	return prefix + hex.EncodeToString(buf), nil
}

// randomNodeID 生成节点标识(n_<16hex>)。
func randomNodeID() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("生成节点 ID: %w", err)
	}
	return "n_" + hex.EncodeToString(buf[:]), nil
}

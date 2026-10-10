// Package handler — agent 数据上报端点(T3.3):远程节点 metrics 采集入库。
package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// AgentReportHandler agent 上报接口。
type AgentReportHandler struct {
	nodes     NodeSecretLookup
	collector *service.MetricsCollector
}

// NewAgentReport 构造 agent 上报 handler。
func NewAgentReport(nodes NodeSecretLookup, collector *service.MetricsCollector) *AgentReportHandler {
	return &AgentReportHandler{nodes: nodes, collector: collector}
}

// NodeSecretLookup 节点密钥反查依赖;由 store 实现。
type NodeSecretLookup interface {
	GetNodeBySecretHash(ctx context.Context, hash string) (*model.Node, error)
}

// ReportAckResponse 上报确认。
type ReportAckResponse struct {
	Samples int `json:"samples"`
}

// reportBodyLimit 上报请求体大小上限(与日志 tail 同级防护)。
const reportBodyLimit = 2 << 20 // 2MiB

// Metrics godoc
// @Summary Agent 上报 metrics
// @Description agent 抓取本机 Caddy /metrics 后上报;认证用节点密钥(Authorization: Bearer cnd_*)
// @Tags agent
// @Accept text/plain
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.ReportAckResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/agent/metrics [post]
func (h *AgentReportHandler) Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID, ok := h.authenticate(c)
		if !ok {
			return
		}
		body, err := io.ReadAll(io.LimitReader(c.Request.Body, reportBodyLimit+1))
		if err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		if len(body) > reportBodyLimit {
			apierr.RespondError(c, http.StatusRequestEntityTooLarge, apierr.CodeBadRequest, i18n.MsgBadRequest,
				map[string]any{"reason": "上报体超过 2MiB 上限"}, nil)
			return
		}
		count, err := h.collector.IngestRemote(c.Request.Context(), nodeID, string(body))
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, err.Error())
			return
		}
		c.JSON(http.StatusOK, ReportAckResponse{Samples: count})
	}
}

// authenticate 节点密钥校验(Bearer cnd_*);失败已写 401 响应。
func (h *AgentReportHandler) authenticate(c *gin.Context) (string, bool) {
	token := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
	if !strings.HasPrefix(token, service.NodeSecretPrefix) {
		unauthorizedNode(c)
		return "", false
	}
	sum := sha256.Sum256([]byte(token))
	node, err := h.nodes.GetNodeBySecretHash(c.Request.Context(), hex.EncodeToString(sum[:]))
	if err != nil {
		unauthorizedNode(c)
		return "", false
	}
	return node.NodeID, true
}

func unauthorizedNode(c *gin.Context) {
	apierr.RespondError(c, http.StatusUnauthorized, apierr.CodeUnauthorized, i18n.MsgUnauthorized, nil, nil)
}

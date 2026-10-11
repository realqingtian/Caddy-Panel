// Package handler — 节点管理接口(T3.3):节点列表/删除、接入令牌与安装命令。
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// NodeHandler 节点管理接口。
type NodeHandler struct {
	nodes *service.Node
}

// NewNode 构造节点管理 handler。
func NewNode(nodes *service.Node) *NodeHandler {
	return &NodeHandler{nodes: nodes}
}

// CreateEnrollTokenBody 签发接入令牌请求体。
type CreateEnrollTokenBody struct {
	NodeName string `json:"node_name" binding:"required" example:"edge-hk-01"`
	Platform string `json:"platform" binding:"required,oneof=linux-amd64 linux-arm64" example:"linux-amd64"`
}

// CreateEnrollTokenResponse 签发响应;令牌明文与安装命令仅此一次。
type CreateEnrollTokenResponse struct {
	PlainToken     string                 `json:"plain_token"`
	InstallCommand string                 `json:"install_command"`
	Token          model.EnrollmentToken  `json:"token"`
}

// UninstallCommandResponse 卸载命令响应。
type UninstallCommandResponse struct {
	Command string                 `json:"command"`
	Steps   []service.UninstallStep `json:"steps"`
}

// List godoc
// @Summary 节点列表
// @Description 返回全部远程节点(local 为面板内置,不入列表)
// @Tags nodes
// @Produce json
// @Security BearerAuth
// @Success 200 {array} model.Node
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/nodes [get]
func (h *NodeHandler) List() gin.HandlerFunc {
	return func(c *gin.Context) {
		nodes, err := h.nodes.ListNodes(c.Request.Context())
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		c.JSON(http.StatusOK, nodes)
	}
}

// Delete godoc
// @Summary 删除节点
// @Description 名下仍有站点时拒绝(先删站点);删除即吊销节点密钥并断开连接
// @Tags nodes
// @Security BearerAuth
// @Param nodeId path string true "节点标识(node_id)"
// @Success 204
// @Failure 400 {object} apierr.ErrorResponse
// @Failure 404 {object} apierr.ErrorResponse
// @Router /api/v1/nodes/{nodeId} [delete]
func (h *NodeHandler) Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("nodeId")
		err := h.nodes.DeleteNode(c.Request.Context(), nodeID)
		if err != nil {
			respondNodeError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// ListEnrollTokens godoc
// @Summary 接入令牌列表
// @Description 返回注册令牌记录(仅哈希与前缀,无明文)
// @Tags nodes
// @Produce json
// @Security BearerAuth
// @Success 200 {array} model.EnrollmentToken
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/nodes/enrollment-tokens [get]
func (h *NodeHandler) ListEnrollTokens() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokens, err := h.nodes.ListEnrollmentTokens(c.Request.Context())
		if err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		c.JSON(http.StatusOK, tokens)
	}
}

// CreateEnrollToken godoc
// @Summary 签发接入令牌并生成安装命令
// @Description 令牌一次性(注册成功即作废,24h 过期);安装命令内嵌令牌,在目标机粘贴执行完成接入
// @Tags nodes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.CreateEnrollTokenBody true "节点名与目标平台"
// @Success 201 {object} handler.CreateEnrollTokenResponse
// @Failure 400 {object} apierr.ErrorResponse
// @Router /api/v1/nodes/enrollment-tokens [post]
func (h *NodeHandler) CreateEnrollToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		var body CreateEnrollTokenBody
		if err := c.ShouldBindJSON(&body); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}
		result, err := h.nodes.CreateEnrollmentToken(c.Request.Context(), body.NodeName, body.Platform)
		if err != nil {
			var invalid *service.InvalidInputError
			if errors.As(err, &invalid) {
				apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest,
					i18n.MsgBadRequest, map[string]any{"reason": invalid.Reason}, invalid.Reason)
				return
			}
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		c.JSON(http.StatusCreated, CreateEnrollTokenResponse{
			PlainToken:     result.PlainToken,
			InstallCommand: result.InstallCommand,
			Token:          *result.Token,
		})
	}
}

// RevokeEnrollToken godoc
// @Summary 撤销接入令牌
// @Description 已使用/已撤销的令牌重复撤销无副作用
// @Tags nodes
// @Security BearerAuth
// @Param id path int true "令牌 ID"
// @Success 204
// @Failure 404 {object} apierr.ErrorResponse
// @Router /api/v1/nodes/enrollment-tokens/{id}/revoke [post]
func (h *NodeHandler) RevokeEnrollToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		if err := h.nodes.RevokeEnrollmentToken(c.Request.Context(), id); err != nil {
			respondNodeError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// DeleteEnrollToken godoc
// @Summary 删除接入令牌记录
// @Tags nodes
// @Security BearerAuth
// @Param id path int true "令牌 ID"
// @Success 204
// @Router /api/v1/nodes/enrollment-tokens/{id} [delete]
func (h *NodeHandler) DeleteEnrollToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		if err := h.nodes.DeleteEnrollmentToken(c.Request.Context(), id); err != nil {
			apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// UninstallCommand godoc
// @Summary 卸载命令
// @Description 生成目标机卸载 agent 的对称命令
// @Tags nodes
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.UninstallCommandResponse
// @Router /api/v1/nodes/uninstall-command [get]
func (h *NodeHandler) UninstallCommand() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, UninstallCommandResponse{Command: service.UninstallCommand(), Steps: service.UninstallSteps()})
	}
}

// respondNodeError 节点业务错误 → 统一错误响应。
func respondNodeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNodeNotFound):
		apierr.RespondError(c, http.StatusNotFound, apierr.CodeNodeNotFound, i18n.MsgNodeNotFound, nil, nil)
	case errors.Is(err, service.ErrNodeHasSites):
		apierr.RespondError(c, http.StatusConflict, apierr.CodeNodeHasSites, i18n.MsgNodeHasSites, nil, err.Error())
	case errors.Is(err, service.ErrNodeNameTaken):
		apierr.RespondError(c, http.StatusConflict, apierr.CodeBadRequest, i18n.MsgBadRequest,
			map[string]any{"reason": err.Error()}, err.Error())
	default:
		var invalid *service.InvalidInputError
		if errors.As(err, &invalid) {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest,
				i18n.MsgBadRequest, map[string]any{"reason": invalid.Reason}, invalid.Reason)
			return
		}
		apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
	}
}

// Package handler — Caddyfile 附加片段接口(logux-design §7)。
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

// SnippetHandler 片段管理接口。
type SnippetHandler struct {
	snippets *service.Snippet
}

// NewSnippet 构造片段 handler。
func NewSnippet(snippets *service.Snippet) *SnippetHandler {
	return &SnippetHandler{snippets: snippets}
}

// SnippetListResponse 片段列表响应。
type SnippetListResponse struct {
	Snippets []model.Snippet `json:"snippets"`
}

// List godoc
// @Summary 片段列表
// @Description 返回全部附加片段(ID 升序 = 渲染追加顺序)
// @Tags snippets
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.SnippetListResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/config/snippets [get]
func (h *SnippetHandler) List() gin.HandlerFunc {
	return func(c *gin.Context) {
		snippets, err := h.snippets.List(c.Request.Context())
		if err != nil {
			respondSnippetError(c, err)
			return
		}
		c.JSON(http.StatusOK, SnippetListResponse{Snippets: snippets})
	}
}

// SaveBody 片段新建/编辑请求体。
type SaveBody struct {
	Name    string `json:"name" example:"rate-limit"`
	Content string `json:"content" example:"(rl) {\n\trate_limit {\n\t\tzone static {\n\t\t\tkey {remote_host}\n\t\t\tevents 100\n\t\t\twindow 1m\n\t\t}\n\t}\n}"`
	Enabled bool   `json:"enabled"`
}

// Create godoc
// @Summary 新建片段
// @Description 校验通过后渲染下发(任一片段非法则整体拒绝),成功才写库(logux-design §7)
// @Tags snippets
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body handler.SaveBody true "片段内容"
// @Success 200 {object} model.Snippet
// @Failure 400 {object} apierr.ErrorResponse "校验失败(如包含全局块)"
// @Failure 409 {object} apierr.ErrorResponse "名称已占用"
// @Failure 422 {object} apierr.ErrorResponse "下发失败(detail 含 Caddy 原文)"
// @Router /api/v1/config/snippets [post]
func (h *SnippetHandler) Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		body, ok := bindSnippetBody(c)
		if !ok {
			return
		}
		snippet, err := h.snippets.Create(c.Request.Context(), body)
		if err != nil {
			respondSnippetError(c, err)
			return
		}
		c.JSON(http.StatusOK, snippet)
	}
}

// Update godoc
// @Summary 编辑片段
// @Description 修改名称/内容/启用开关;变更经管线下发成功后才生效(logux-design §7)
// @Tags snippets
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "片段 ID"
// @Param body body handler.SaveBody true "片段内容"
// @Success 200 {object} model.Snippet
// @Failure 400 {object} apierr.ErrorResponse "校验失败(如包含全局块)"
// @Failure 404 {object} apierr.ErrorResponse
// @Failure 409 {object} apierr.ErrorResponse "名称已占用"
// @Failure 422 {object} apierr.ErrorResponse "下发失败(detail 含 Caddy 原文)"
// @Router /api/v1/config/snippets/{id} [put]
func (h *SnippetHandler) Update() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		body, ok := bindSnippetBody(c)
		if !ok {
			return
		}
		snippet, err := h.snippets.Update(c.Request.Context(), id, body)
		if err != nil {
			respondSnippetError(c, err)
			return
		}
		c.JSON(http.StatusOK, snippet)
	}
}

// Delete godoc
// @Summary 删除片段
// @Description 启用片段先重下发移除产物块,再删记录(logux-design §7)
// @Tags snippets
// @Produce json
// @Security BearerAuth
// @Param id path int true "片段 ID"
// @Success 204 {string} string
// @Failure 404 {object} apierr.ErrorResponse
// @Failure 422 {object} apierr.ErrorResponse "下发失败(detail 含 Caddy 原文)"
// @Router /api/v1/config/snippets/{id} [delete]
func (h *SnippetHandler) Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		if err := h.snippets.Delete(c.Request.Context(), id); err != nil {
			respondSnippetError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// bindSnippetBody 解析片段请求体;失败时已写响应。
func bindSnippetBody(c *gin.Context) (service.SnippetInput, bool) {
	var body SaveBody
	if err := c.ShouldBindJSON(&body); err != nil {
		apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
		return service.SnippetInput{}, false
	}
	return service.SnippetInput{Name: body.Name, Content: body.Content, Enabled: body.Enabled}, true
}

// respondSnippetError 片段业务错误 → 统一错误响应。
func respondSnippetError(c *gin.Context, err error) {
	var invalid *service.InvalidInputError
	var deploy *service.DeployError
	switch {
	case errors.As(err, &invalid):
		apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest,
			i18n.MsgBadRequest, map[string]any{"reason": invalid.Reason}, invalid.Reason)
	case errors.Is(err, service.ErrSnippetNameTaken):
		apierr.RespondError(c, http.StatusConflict, apierr.CodeSnippetNameTaken,
			i18n.MsgSnippetNameTaken, nil, nil)
	case errors.Is(err, service.ErrSnippetNotFound):
		apierr.RespondError(c, http.StatusNotFound, apierr.CodeNotFound, i18n.MsgNotFound, nil, nil)
	case errors.As(err, &deploy):
		apierr.RespondError(c, http.StatusUnprocessableEntity, apierr.CodeConfigDeployFailed,
			i18n.MsgConfigDeployFailed, nil, deploy.Error())
	default:
		apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
	}
}

// Package handler — 站点管理接口:仅协议转换,业务在 service 层。
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"caddy-panel/backend/internal/api/middleware"
	"caddy-panel/backend/internal/apierr"
	"caddy-panel/backend/internal/i18n"
	"caddy-panel/backend/internal/model"
	"caddy-panel/backend/internal/service"
)

// SiteHandler 站点管理接口。
type SiteHandler struct {
	sites *service.Site
}

// NewSite 构造站点 handler。
func NewSite(sites *service.Site) *SiteHandler {
	return &SiteHandler{sites: sites}
}

// SiteMutationResponse 站点写操作响应;applied 表示是否已热加载生效
// (dry run 时 site 未落库、applied 恒为 false)。
type SiteMutationResponse struct {
	Site    model.Site `json:"site"`
	Applied bool       `json:"applied"`
}

// SiteListResponse 站点列表响应。
type SiteListResponse struct {
	Sites []model.Site `json:"sites"`
}

// Create godoc
// @Summary 新建站点
// @Description 校验并下发新站点配置;dry_run=true 时仅预检,不落库不生效(FR-301/304)
// @Tags sites
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body service.SiteInput true "站点配置"
// @Param dry_run query bool false "仅预检不落库" default(false)
// @Success 201 {object} handler.SiteMutationResponse
// @Failure 400 {object} apierr.ErrorResponse "输入校验失败"
// @Failure 409 {object} apierr.ErrorResponse "域名冲突"
// @Failure 422 {object} apierr.ErrorResponse "校验或下发失败(detail 含 Caddy 原文)"
// @Router /api/v1/sites [post]
func (h *SiteHandler) Create() gin.HandlerFunc {
	return func(c *gin.Context) {
		var in service.SiteInput
		if err := c.ShouldBindJSON(&in); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}

		site, result, err := h.sites.Create(c.Request.Context(), in, c.Query("dry_run") == "true")
		if err != nil {
			respondSiteError(c, err)
			return
		}
		c.Set(middleware.CtxKeyAuditTarget, site.Name)
		c.JSON(http.StatusCreated, SiteMutationResponse{Site: *site, Applied: result.Applied})
	}
}

// List godoc
// @Summary 站点列表
// @Tags sites
// @Produce json
// @Security BearerAuth
// @Success 200 {object} handler.SiteListResponse
// @Failure 401 {object} apierr.ErrorResponse
// @Router /api/v1/sites [get]
func (h *SiteHandler) List() gin.HandlerFunc {
	return func(c *gin.Context) {
		sites, err := h.sites.List(c.Request.Context())
		if err != nil {
			respondSiteError(c, err)
			return
		}
		c.JSON(http.StatusOK, SiteListResponse{Sites: sites})
	}
}

// Get godoc
// @Summary 站点详情
// @Tags sites
// @Produce json
// @Security BearerAuth
// @Param id path int true "站点 ID"
// @Success 200 {object} model.Site
// @Failure 404 {object} apierr.ErrorResponse
// @Router /api/v1/sites/{id} [get]
func (h *SiteHandler) Get() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		site, err := h.sites.Get(c.Request.Context(), id)
		if err != nil {
			respondSiteError(c, err)
			return
		}
		c.JSON(http.StatusOK, site)
	}
}

// Update godoc
// @Summary 更新站点
// @Description 校验并下发更新后的全量配置(FR-305)
// @Tags sites
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "站点 ID"
// @Param body body service.SiteInput true "站点配置"
// @Success 200 {object} handler.SiteMutationResponse
// @Failure 404 {object} apierr.ErrorResponse
// @Failure 409 {object} apierr.ErrorResponse "域名冲突"
// @Failure 422 {object} apierr.ErrorResponse "校验或下发失败"
// @Router /api/v1/sites/{id} [put]
func (h *SiteHandler) Update() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		var in service.SiteInput
		if err := c.ShouldBindJSON(&in); err != nil {
			apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest, nil, err.Error())
			return
		}

		site, result, err := h.sites.Update(c.Request.Context(), id, in)
		if err != nil {
			respondSiteError(c, err)
			return
		}
		c.Set(middleware.CtxKeyAuditTarget, site.Name)
		c.JSON(http.StatusOK, SiteMutationResponse{Site: *site, Applied: result.Applied})
	}
}

// Enable godoc
// @Summary 启用站点
// @Tags sites
// @Produce json
// @Security BearerAuth
// @Param id path int true "站点 ID"
// @Success 200 {object} handler.SiteMutationResponse
// @Failure 404 {object} apierr.ErrorResponse
// @Failure 422 {object} apierr.ErrorResponse
// @Router /api/v1/sites/{id}/enable [post]
func (h *SiteHandler) Enable() gin.HandlerFunc {
	return h.setEnabled(true)
}

// Disable godoc
// @Summary 停用站点(域名随渲染产物下线,FR-305)
// @Tags sites
// @Produce json
// @Security BearerAuth
// @Param id path int true "站点 ID"
// @Success 200 {object} handler.SiteMutationResponse
// @Failure 404 {object} apierr.ErrorResponse
// @Failure 422 {object} apierr.ErrorResponse
// @Router /api/v1/sites/{id}/disable [post]
func (h *SiteHandler) Disable() gin.HandlerFunc {
	return h.setEnabled(false)
}

func (h *SiteHandler) setEnabled(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		site, result, err := h.sites.SetEnabled(c.Request.Context(), id, enabled)
		if err != nil {
			respondSiteError(c, err)
			return
		}
		c.Set(middleware.CtxKeyAuditTarget, site.Name)
		c.JSON(http.StatusOK, SiteMutationResponse{Site: *site, Applied: result.Applied})
	}
}

// Delete godoc
// @Summary 删除站点
// @Description 下发不含该站点的全量配置,成功后删除记录(FR-305)
// @Tags sites
// @Security BearerAuth
// @Param id path int true "站点 ID"
// @Success 204 "已删除并生效"
// @Failure 404 {object} apierr.ErrorResponse
// @Failure 422 {object} apierr.ErrorResponse
// @Router /api/v1/sites/{id} [delete]
func (h *SiteHandler) Delete() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := parseID(c)
		if !ok {
			return
		}
		if _, err := h.sites.Delete(c.Request.Context(), id); err != nil {
			respondSiteError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// parseID 解析路径参数 id;非法时直接写 400 响应。
func parseID(c *gin.Context) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id64 == 0 {
		apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest,
			map[string]any{"reason": "站点 ID 非法"}, nil)
		return 0, false
	}
	return uint(id64), true
}

// respondSiteError 站点业务错误 → 统一错误响应;
// 下发失败(detail)保留 Caddy 错误原文原样透传(FR-304/405)。
func respondSiteError(c *gin.Context, err error) {
	var invalid *service.InvalidInputError
	var conflict *service.DomainConflictError
	var deploy *service.DeployError
	switch {
	case errors.As(err, &invalid):
		apierr.RespondError(c, http.StatusBadRequest, apierr.CodeBadRequest, i18n.MsgBadRequest,
			map[string]any{"reason": invalid.Reason}, invalid.Reason)
	case errors.As(err, &conflict):
		apierr.RespondError(c, http.StatusConflict, apierr.CodeSiteDomainConflict, i18n.MsgSiteDomainConflict,
			map[string]any{"domain": conflict.Domain}, nil)
	case errors.As(err, &deploy):
		apierr.RespondError(c, http.StatusUnprocessableEntity, apierr.CodeSiteDeployFailed, i18n.MsgSiteDeployFailed,
			nil, deploy.Error())
	case errors.Is(err, service.ErrSiteNotFound):
		apierr.RespondError(c, http.StatusNotFound, apierr.CodeSiteNotFound, i18n.MsgSiteNotFound, nil, nil)
	default:
		apierr.RespondError(c, http.StatusInternalServerError, apierr.CodeInternal, i18n.MsgInternal, nil, nil)
	}
}

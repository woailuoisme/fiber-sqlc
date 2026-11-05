package v1

import (
	"github.com/gofiber/fiber/v3"

	"cfiber/internal/middleware"
	"cfiber/pkg/authorization"
)

// RegisterRoutes 在后台路由组下挂载用户运营管理端点 (/admin/users)。
func RegisterRoutes(router fiber.Router, h *Handler, mw *middleware.Middleware) {
	group := router.Group("/admin/users", mw.Auth("admin"), mw.Throttle(middleware.TierAuthenticated))

	group.Get("", mw.Can(authorization.ResourceUsers, authorization.ActionList), h.ListUsers)
	group.Get("/export", mw.Can(authorization.ResourceUsers, authorization.ActionList), h.ExportUsers)
	group.Post("", mw.Can(authorization.ResourceUsers, authorization.ActionCreate), h.CreateUser)
	group.Get("/:id<guid>", mw.Can(authorization.ResourceUsers, authorization.ActionRead), h.GetUser)
	group.Patch("/:id<guid>", mw.Can(authorization.ResourceUsers, authorization.ActionUpdate), h.UpdateUser)
	group.Delete("/:id<guid>", mw.Can(authorization.ResourceUsers, authorization.ActionDelete), h.DeleteUser)
}

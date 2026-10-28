package adminroute

import (
	"github.com/gofiber/fiber/v3"

	adminhandler "cfiber/internal/handler/admin"
	"cfiber/internal/middleware"
	"cfiber/pkg/authorization"
)

// RegisterAdminUserRoutes 注册后台用户管理端点（/v1/admin/users）。
// 每个端点各自声明所需权限码，权限矩阵见 pkg/authorization/rbac/policy.csv。
func RegisterAdminUserRoutes(router fiber.Router, h *adminhandler.UserHandler, mw *middleware.Middleware) {
	group := router.Group("/admin/users", mw.Auth("admin"), mw.Throttle(middleware.TierAuthenticated))

	group.Get("", mw.Can(authorization.ResourceUsers, authorization.ActionList), h.ListUsers)
	group.Get("/export", mw.Can(authorization.ResourceUsers, authorization.ActionList), h.ExportUsers)
	group.Post("", mw.Can(authorization.ResourceUsers, authorization.ActionCreate), h.CreateUser)
	group.Get("/:id<guid>", mw.Can(authorization.ResourceUsers, authorization.ActionRead), h.GetUser)
	group.Patch("/:id<guid>", mw.Can(authorization.ResourceUsers, authorization.ActionUpdate), h.UpdateUser)
	group.Delete("/:id<guid>", mw.Can(authorization.ResourceUsers, authorization.ActionDelete), h.DeleteUser)
}

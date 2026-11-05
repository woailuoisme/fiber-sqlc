package v1

import (
	"github.com/gofiber/fiber/v3"

	"cfiber/internal/middleware"
)

// RegisterRoutes 在后台路由组下挂载管理员认证相关端点 (/admin/auth)。
func RegisterRoutes(router fiber.Router, h *Handler, mw *middleware.Middleware) {
	group := router.Group("/admin/auth")

	tierSensitive := mw.Throttle(middleware.TierSensitive)
	group.Post("/sign-in/email", tierSensitive, h.SignInEmail)
	group.Post("/token/refresh", tierSensitive, h.Refresh)

	adminAuth := mw.Auth("admin")
	tierAuth := mw.Throttle(middleware.TierAuthenticated)
	group.Get("/get-session", adminAuth, tierAuth, h.GetSession)
	group.Post("/sign-out", adminAuth, tierAuth, h.SignOut)
	group.Post("/change-password", adminAuth, tierAuth, h.ChangePassword)
}

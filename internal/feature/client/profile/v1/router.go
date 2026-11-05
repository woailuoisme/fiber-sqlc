package v1

import (
	"github.com/gofiber/fiber/v3"

	"cfiber/internal/middleware"
)

// RegisterRoutes 在指定路由器下挂载用户个人资料端点 (/users)。
func RegisterRoutes(router fiber.Router, h *Handler, mw *middleware.Middleware) {
	usersGroup := router.Group("/users", mw.Auth("client"), mw.Throttle(middleware.TierAuthenticated))
	usersGroup.Get("/me", h.GetProfile)
	usersGroup.Patch("/me", h.UpdateProfile)
	usersGroup.Post("/me/avatar", h.UploadAvatar)
}

package route

import (
	"github.com/gofiber/fiber/v3"

	"cfiber/internal/handler"
	"cfiber/internal/middleware"
)

// RegisterProfileRoutes 注册前端用户个人档案端点 (/v1/users)。
func RegisterProfileRoutes(router fiber.Router, h *handler.ProfileHandler, mw *middleware.Middleware) {
	group := router.Group("/users", mw.Auth("client"), mw.Throttle(middleware.TierAuthenticated))

	group.Get("/me", h.GetProfile)
	group.Patch("/me", h.UpdateProfile)
	group.Post("/me/avatar", h.UploadAvatar)
}

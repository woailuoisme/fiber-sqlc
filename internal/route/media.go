package route

import (
	"github.com/gofiber/fiber/v3"

	"cfiber/internal/handler"
	"cfiber/internal/middleware"
)

// RegisterMediaRoutes 注册媒体相关端点 (/v1/media)。
func RegisterMediaRoutes(router fiber.Router, h *handler.MediaHandler, mw *middleware.Middleware) {
	group := router.Group("/media", mw.Auth("client"), mw.Throttle(middleware.TierAuthenticated))

	group.Post("/presign", h.Presign)
}

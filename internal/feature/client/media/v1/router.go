package v1

import (
	"github.com/gofiber/fiber/v3"

	"cfiber/internal/middleware"
)

// RegisterRoutes 在指定路由器下挂载媒体端点 (/media)。
func RegisterRoutes(router fiber.Router, h *Handler, mw *middleware.Middleware) {
	mediaGroup := router.Group("/media", mw.Auth("client"), mw.Throttle(middleware.TierAuthenticated))
	mediaGroup.Post("/presign", h.Presign)
}

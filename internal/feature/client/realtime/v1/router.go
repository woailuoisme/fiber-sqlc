package v1

import (
	"github.com/gofiber/fiber/v3"

	"cfiber/internal/middleware"
)

// RegisterRoutes 在指定路由器下挂载实时通信端点 (/realtime)。
func RegisterRoutes(router fiber.Router, h *Handler, mw *middleware.Middleware) {
	realtimeGroup := router.Group("/realtime")
	realtimeGroup.Get("/token", mw.Auth("client"), mw.Throttle(middleware.TierAuthenticated), h.GetToken)
	realtimeGroup.Post("/proxy/subscribe", h.ProxySubscribe)
}

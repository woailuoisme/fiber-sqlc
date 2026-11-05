package v1

import (
	"github.com/gofiber/fiber/v3"

	"cfiber/internal/middleware"
)

// RegisterRoutes 在指定路由器下挂载支付端点 (/payments)。
func RegisterRoutes(router fiber.Router, h *Handler, _ *middleware.Middleware) {
	paymentsGroup := router.Group("/payments")
	paymentsGroup.Post("/notify/:channel", h.HandleNotification)
	paymentsGroup.Post("/notify/:channel/:app", h.HandleNotification)
	paymentsGroup.Get("/return/alipay", h.HandleAlipayReturn)
}

package client

import (
	"github.com/gofiber/fiber/v3"

	authv1 "cfiber/internal/feature/client/auth/v1"
	mediav1 "cfiber/internal/feature/client/media/v1"
	paymentv1 "cfiber/internal/feature/client/payment/v1"
	profilev1 "cfiber/internal/feature/client/profile/v1"
	realtimev1 "cfiber/internal/feature/client/realtime/v1"
	"cfiber/internal/middleware"
)

// Handlers 聚合 client 端内各业务特性的 HTTP 处理器。
type Handlers struct {
	Auth     *authv1.Handler
	Profile  *profilev1.Handler
	Payment  *paymentv1.Handler
	Media    *mediav1.Handler
	Realtime *realtimev1.Handler
}

// RegisterRoutes 将客户端所有业务特性的 v1 路由挂载至客户端路由器（如 /v1）。
func RegisterRoutes(v1 fiber.Router, h Handlers, mw *middleware.Middleware) {
	authv1.RegisterRoutes(v1, h.Auth, mw)
	profilev1.RegisterRoutes(v1, h.Profile, mw)
	paymentv1.RegisterRoutes(v1, h.Payment, mw)
	mediav1.RegisterRoutes(v1, h.Media, mw)
	realtimev1.RegisterRoutes(v1, h.Realtime, mw)
}

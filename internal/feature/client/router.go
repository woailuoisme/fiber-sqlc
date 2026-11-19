package client

import (
	"github.com/gofiber/fiber/v3"

	clientauth "cfiber/internal/feature/client/auth"
	clientmedia "cfiber/internal/feature/client/media"
	clientpayment "cfiber/internal/feature/client/payment"
	clientprofile "cfiber/internal/feature/client/profile"
	clientrealtime "cfiber/internal/feature/client/realtime"
	"cfiber/internal/middleware"
)

// Handlers 聚合 client 端内各业务特性的 HTTP 处理器。
type Handlers struct {
	Auth     *clientauth.Handler
	Profile  *clientprofile.Handler
	Payment  *clientpayment.Handler
	Media    *clientmedia.Handler
	Realtime *clientrealtime.Handler
}

// RegisterRoutes 将客户端所有业务特性的 v1 路由挂载至客户端路由器（如 /v1）。
func RegisterRoutes(v1 fiber.Router, h Handlers, mw *middleware.Middleware) {
	clientauth.RegisterRoutes(v1, h.Auth, mw)
	clientprofile.RegisterRoutes(v1, h.Profile, mw)
	clientpayment.RegisterRoutes(v1, h.Payment, mw)
	clientmedia.RegisterRoutes(v1, h.Media, mw)
	clientrealtime.RegisterRoutes(v1, h.Realtime, mw)
}

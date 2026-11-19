package admin

import (
	"github.com/gofiber/fiber/v3"

	adminauth "cfiber/internal/feature/admin/auth"
	adminuser "cfiber/internal/feature/admin/user"
	"cfiber/internal/middleware"
)

// Handlers 聚合管理端各业务特性的 HTTP 处理器。
type Handlers struct {
	Auth *adminauth.Handler
	User *adminuser.Handler
}

// RegisterRoutes 将管理端所有业务特性的 v1 路由挂载至管理端路由器（如 /v1）。
func RegisterRoutes(v1 fiber.Router, h Handlers, mw *middleware.Middleware) {
	adminauth.RegisterRoutes(v1, h.Auth, mw)
	adminuser.RegisterRoutes(v1, h.User, mw)
}

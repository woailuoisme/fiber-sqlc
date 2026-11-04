// Package middleware 提供路由级传输层中间件（认证守卫、RBAC 鉴权与分级滑动窗口限流）。
// 全局引擎级中间件位于子包 internal/middleware/global。
package middleware

import (
	"github.com/gofiber/fiber/v3"

	"cfiber/internal/config"
	"cfiber/pkg/authentication"
	"cfiber/pkg/authorization"
	"cfiber/pkg/logger"
)

// Deps 声明路由级中间件所需的外部依赖。
type Deps struct {
	AuthManager *authentication.Manager
	Storage     fiber.Storage
	Authz       *authorization.Enforcer
	Logger      *logger.Logger
	Limiter     config.LimiterConfig
}

// Middleware 聚合路由级中间件工厂方法。
type Middleware struct {
	authManager *authentication.Manager
	storage     fiber.Storage
	authz       *authorization.Enforcer
	log         *logger.Logger
	limiter     config.LimiterConfig
}

// New 构造路由级中间件管理器。
func New(deps Deps) *Middleware {
	log := deps.Logger
	if log == nil {
		log = logger.Nop()
	}
	return &Middleware{
		authManager: deps.AuthManager,
		storage:     deps.Storage,
		authz:       deps.Authz,
		log:         log,
		limiter:     deps.Limiter,
	}
}

// SetLimiterStorage 替换限流中间件使用的 fiber.Storage 存储后端（主要供测试注入模拟存储）。
func (m *Middleware) SetLimiterStorage(storage fiber.Storage) {
	m.storage = storage
}

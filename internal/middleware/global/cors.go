package global

import (
	"slices"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
)

// CORS 【全局中间件】基于 fiber/v3/middleware/cors 统一处理跨域资源共享与 OPTIONS 预检请求。
func (m *Middleware) CORS() fiber.Handler {
	cfg := cors.Config{
		AllowMethods: []string{
			fiber.MethodGet,
			fiber.MethodPost,
			fiber.MethodPatch,
			fiber.MethodPut,
			fiber.MethodDelete,
			fiber.MethodHead,
			fiber.MethodOptions,
		},
		AllowHeaders: []string{
			fiber.HeaderAuthorization,
			fiber.HeaderContentType,
			fiber.HeaderAccept,
			"x-request-id",
			"Idempotency-Key",
			fiber.HeaderAcceptLanguage,
		},
		ExposeHeaders: []string{
			"x-request-id",
			"X-RateLimit-Limit",
			"X-RateLimit-Remaining",
			"X-RateLimit-Reset",
			fiber.HeaderRetryAfter,
		},
		MaxAge: int((12 * time.Hour).Seconds()),
	}

	switch {
	case slices.Contains(m.cors.AllowedOrigins, "*"):
		cfg.AllowOrigins = []string{"*"}
		cfg.AllowCredentials = false // 遵循 W3C 规范：通配符禁止回显允许凭证
	case len(m.cors.AllowedOrigins) > 0:
		cfg.AllowOrigins = m.cors.AllowedOrigins
		cfg.AllowCredentials = true
	default:
		return func(c fiber.Ctx) error {
			return c.Next()
		}
	}

	return cors.New(cfg)
}

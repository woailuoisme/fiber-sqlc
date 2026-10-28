package global

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/favicon"
)

// Favicon 拦截浏览器和爬虫静默 /favicon.ico 请求，避免产生 404 噪音与路由开销。
func (m *Middleware) Favicon() fiber.Handler {
	return favicon.New()
}

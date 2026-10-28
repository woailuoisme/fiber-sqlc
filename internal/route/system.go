package route

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/healthcheck"

	"cfiber/internal/handler"
)

// RegisterSystemRoutes 注册根路径与系统健康探测端点，统一基于官方 healthcheck 端点契约。
func RegisterSystemRoutes(app *fiber.App, h *handler.SystemHandler) {
	app.Get("/", h.Index)
	app.Get(healthcheck.LivenessEndpoint, h.Liveness)
	app.Get(healthcheck.ReadinessEndpoint, h.Readiness)
}

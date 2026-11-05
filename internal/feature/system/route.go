package system

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/healthcheck"
)

// RegisterRoutes 注册根路径与系统健康探测端点，统一基于官方 healthcheck 端点契约。
func RegisterRoutes(app *fiber.App, h *SystemHandler) {
	app.Get("/", h.Index)
	app.Get(healthcheck.LivenessEndpoint, h.Liveness)
	app.Get(healthcheck.ReadinessEndpoint, h.Readiness)
}

package system

import (
	"cmp"
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/healthcheck"

	"cfiber/pkg/logger"
	"cfiber/pkg/ready"
	"cfiber/pkg/response"
)

// probeTimeout 是就绪探针调度的总超时时限。
const probeTimeout = 2 * time.Second

var livenessHandler = healthcheck.New()

// SystemHandler 是探针端点的 HTTP 适配层。
type SystemHandler struct {
	svc    *SystemService
	logger *logger.Logger
}

// NewSystemHandler 构造系统探针 SystemHandler，日志器未注入时静默兜底。
func NewSystemHandler(svc *SystemService, log *logger.Logger) *SystemHandler {
	return &SystemHandler{svc: svc, logger: cmp.Or(log, logger.Nop())}
}

// Index 处理 GET / 根路径请求。
func (h *SystemHandler) Index(c fiber.Ctx) error {
	return h.Liveness(c)
}

// Liveness 基于官方 healthcheck 中间件处理 GET /livez 存活探针请求。
func (h *SystemHandler) Liveness(c fiber.Ctx) error {
	return livenessHandler(c)
}

// Readiness 处理 GET /readyz 就绪探针请求：并发探测各依赖，关键项失败返回 503，非关键项失败返回 200 degraded。
func (h *SystemHandler) Readiness(c fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), probeTimeout)
	defer cancel()

	result, errs := h.svc.Check(ctx)
	for name, err := range errs {
		h.logger.WarnContext(ctx, "readiness probe failed",
			slog.Any("error", err),
			slog.String("component", name),
		)
	}

	if result.Status == ready.StatusNotReady {
		return c.Status(result.StatusCode()).JSON(result)
	}
	return response.OK(c, result)
}

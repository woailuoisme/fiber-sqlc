package global

import (
	"github.com/gofiber/contrib/v3/otel"
	"github.com/gofiber/fiber/v3"

	"cfiber/pkg/telemetry"
)

const headerTraceID = "x-trace-id"

// Telemetry 基于官方 contrib/v3/otel 负责 HTTP 链路追踪上下文提取/注入与请求指标采集。
func (m *Middleware) Telemetry() fiber.Handler {
	return otel.New(
		otel.WithNext(func(c fiber.Ctx) bool {
			return !telemetry.IsInitialized() || isProbeRequest(c)
		}),
		otel.WithTraceResponseHeader(headerTraceID),
		otel.WithClientIP(true),
		otel.WithSpanNameFormatter(func(c fiber.Ctx) string {
			return c.Method() + " " + c.Path()
		}),
	)
}

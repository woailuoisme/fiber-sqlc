package global

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/timeout"

	"cfiber/pkg/errs"
)

// Timeout 基于 fiber/v3/middleware/timeout 控制请求超时，超时输出 504，长连接自动豁免。
func (m *Middleware) Timeout() fiber.Handler {
	if m.requestTimeout <= 0 {
		return func(c fiber.Ctx) error {
			return c.Next()
		}
	}

	return timeout.New(func(c fiber.Ctx) error {
		return c.Next()
	}, timeout.Config{
		Timeout: m.requestTimeout,
		Next:    isStreamingRequest,
		OnTimeout: func(c fiber.Ctx) error {
			_ = errs.Render(c, errs.GatewayTimeout(context.DeadlineExceeded))
			return fiber.NewError(fiber.StatusGatewayTimeout, "GATEWAY_TIMEOUT")
		},
	})
}

// isStreamingRequest 判断是否为 WebSocket 握手或 SSE 长连接，这类请求豁免超时。
func isStreamingRequest(c fiber.Ctx) bool {
	return strings.EqualFold(c.Get("Upgrade"), "websocket") ||
		strings.Contains(c.Get("Accept"), "text/event-stream")
}

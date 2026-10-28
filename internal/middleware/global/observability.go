package global

import (
	"cmp"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofiber/utils/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"cfiber/pkg/errs"
	"cfiber/pkg/logger"
)

// headerRequestID 是全链路追踪请求头。
const headerRequestID = "x-request-id"

// requestIDPrefix 是追踪标识前缀。
const requestIDPrefix = "req_"

// RequestID 基于 UUIDv7 / OTel TraceID 生成或透传全链路请求标识。
func (m *Middleware) RequestID() fiber.Handler {
	return func(c fiber.Ctx) error {
		// 探针无外部追踪头时跳过，省掉 UUIDv7 生成与子日志器分配。
		if isProbeRequest(c) && strings.TrimSpace(c.Get(headerRequestID)) == "" {
			return c.Next()
		}

		rid := utils.CopyString(strings.TrimSpace(c.Get(headerRequestID)))
		if rid == "" {
			if span := trace.SpanFromContext(c.Context()); span.SpanContext().IsValid() {
				rid = span.SpanContext().TraceID().String()
			} else {
				rid = newRequestID()
			}
		}

		c.Set(headerRequestID, rid)
		c.Locals(headerRequestID, rid)
		c.SetContext(logger.WithContext(c.Context(), m.logger, rid))
		return c.Next()
	}
}

// newRequestID 生成形如 req_<uuidv7> 的追踪标识。
func newRequestID() string {
	id, err := uuid.NewV7()
	if err != nil {
		return requestIDPrefix + strings.ReplaceAll(uuid.NewString(), "-", "")
	}

	buf := make([]byte, 0, len(requestIDPrefix)+hex.EncodedLen(len(id)))
	buf = append(buf, requestIDPrefix...)
	return string(hex.AppendEncode(buf, id[:]))
}

// AccessLog 记录单行访问日志：方法、路径、状态码、耗时、来源 IP 与请求标识。
func (m *Middleware) AccessLog() fiber.Handler {
	baseLog := cmp.Or(m.logger, logger.Default())

	return func(c fiber.Ctx) error {
		start := time.Now()
		chainErr := c.Next()

		if chainErr != nil {
			if err := c.App().ErrorHandler(c, chainErr); err != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}

		status := c.Response().StatusCode()
		// 探针 2xx/3xx 静默避免心跳刷屏，异常仍记录。
		if isProbeRequest(c) && status < fiber.StatusBadRequest {
			return nil
		}

		event := baseLog.Info()
		switch {
		case status >= fiber.StatusInternalServerError:
			event = baseLog.Error()
		case status >= fiber.StatusBadRequest:
			event = baseLog.Warn()
		}

		if rid := requestIDOf(c); rid != "" {
			event = event.Str("request_id", rid)
		}
		if chainErr != nil {
			event = event.Str("error", chainErr.Error())
		}
		if code := errs.ErrorCodeFromContext(c); code != "" {
			event = event.Str("code", code)
		}

		event.
			Str("method", c.Method()).
			Str("path", c.Path()).
			Int("status", status).
			Str("latency", time.Since(start).String()).
			Str("ip", c.IP()).
			Msg("access")

		return nil
	}
}

// requestIDOf 优先取响应头回写的追踪标识，回退到 requestid 中间件的上下文值。
func requestIDOf(c fiber.Ctx) string {
	if rid := strings.TrimSpace(c.GetRespHeader(headerRequestID)); rid != "" {
		return rid
	}
	return requestid.FromContext(c)
}

// isProbeRequest 判断是否为高频探针路由。
func isProbeRequest(c fiber.Ctx) bool {
	return isProbePath(c.Path())
}

// isProbePath 判断请求是否为高频系统探针路由。
func isProbePath(path string) bool {
	return path == "/livez" || path == "/readyz" || path == "/healthz"
}

// Recovery 捕获 panic 转为统一 500 信封，堆栈仅进日志。
func (m *Middleware) Recovery() fiber.Handler {
	return recover.New(recover.Config{
		EnableStackTrace: true,
		StackTraceHandler: func(c fiber.Ctx, e any) {
			logger.FromContext(c.Context()).Error().
				Any("panic", e).
				Msg("request panic recovered")
		},
	})
}

package middleware_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cfiber/internal/middleware/global"
)

// TestTimeoutMiddlewareWithinBudget 验证请求在超时预算内正常返回。
func TestTimeoutMiddlewareWithinBudget(t *testing.T) {
	m := global.New(global.Deps{
		RequestTimeout: 50 * time.Millisecond,
	})

	app := fiber.New(fiber.Config{
		ErrorHandler: global.HandleError,
	})
	app.Use(m.Timeout())
	app.Get("/fast", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/fast", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestTimeoutMiddlewareExceeded 验证超出截止期时返回 504 GATEWAY_TIMEOUT。
func TestTimeoutMiddlewareExceeded(t *testing.T) {
	m := global.New(global.Deps{
		RequestTimeout: 20 * time.Millisecond,
	})

	app := fiber.New(fiber.Config{
		ErrorHandler: global.HandleError,
	})
	app.Use(m.Timeout())
	app.Get("/slow", func(c fiber.Ctx) error {
		time.Sleep(100 * time.Millisecond)
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "late"})
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusGatewayTimeout, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var res map[string]any
	require.NoError(t, json.Unmarshal(body, &res))
	assert.Equal(t, "GATEWAY_TIMEOUT", res["code"])
}

// TestTimeoutMiddleware_StreamingBypass 验证 WebSocket 升级与 SSE 流式请求自动豁免固定超时。
func TestTimeoutMiddleware_StreamingBypass(t *testing.T) {
	m := global.New(global.Deps{
		RequestTimeout: 20 * time.Millisecond,
	})

	app := fiber.New(fiber.Config{
		ErrorHandler: global.HandleError,
	})
	app.Use(m.Timeout())
	app.Get("/stream", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "streaming"})
	})

	// 1. 验证 WebSocket Upgrade 标头豁免
	reqWS := httptest.NewRequest(http.MethodGet, "/stream", nil)
	reqWS.Header.Set("Upgrade", "websocket")
	respWS, err := app.Test(reqWS)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, respWS.StatusCode)

	// 2. 验证 SSE Accept: text/event-stream 标头豁免
	reqSSE := httptest.NewRequest(http.MethodGet, "/stream", nil)
	reqSSE.Header.Set("Accept", "text/event-stream")
	respSSE, err := app.Test(reqSSE)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, respSSE.StatusCode)
}

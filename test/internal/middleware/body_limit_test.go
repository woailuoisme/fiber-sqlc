package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cfiber/internal/middleware/global"
	"cfiber/pkg/errs"
)

// TestBodyLimit 锁定请求体上限这条安全控制的两端：
// 超限的读取必须被映射成 413 REQUEST_TOO_LARGE，
// 上限之内的请求必须照常通过。
func TestBodyLimit(t *testing.T) {
	mw := global.New(global.Deps{})

	newApp := func() *fiber.App {
		app := fiber.New(fiber.Config{
			ErrorHandler: global.HandleError,
		})
		app.Use(mw.BodyLimit())
		app.Post("/echo", func(c fiber.Ctx) error {
			body := c.Body()
			return c.Status(http.StatusOK).JSON(fiber.Map{"size": len(body)})
		})
		return app
	}

	t.Run("超过上限返回 413", func(t *testing.T) {
		oversized := strings.Repeat("a", 2<<20)
		req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(oversized))
		req.Header.Set("Content-Type", "text/plain")

		resp, err := newApp().Test(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		require.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
		assert.Contains(t, string(body), string(errs.CodeRequestTooLarge))
	})

	t.Run("上限之内正常通过", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"ok":true}`))
		req.Header.Set("Content-Type", "application/json")

		resp, err := newApp().Test(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.JSONEq(t, `{"size":11}`, string(body))
	})
}

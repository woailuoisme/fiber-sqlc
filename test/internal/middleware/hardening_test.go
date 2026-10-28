package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cfiber/internal/middleware/global"
)

// TestRecovery 锁定 panic 边界的对外行为：状态码统一为 500，响应体不带任何内部细节。
func TestRecovery(t *testing.T) {
	mw := global.New(global.Deps{})

	app := fiber.New(fiber.Config{
		ErrorHandler: global.HandleError,
	})
	app.Use(mw.Recovery())
	app.Get("/boom", func(_ fiber.Ctx) error {
		panic("secret-internal-detail")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Contains(t, string(body), "INTERNAL_SERVER_ERROR")
	assert.NotContains(t, string(body), "secret-internal-detail",
		"panic 详情只能进日志，绝不能出现在响应体里")
}

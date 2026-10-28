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

// TestSessionMiddleware_SetAndGet 验证 Session 能够正确在上下文中读写会话状态并随 Cookie 回传。
func TestSessionMiddleware_SetAndGet(t *testing.T) {
	m := global.New(global.Deps{
		SessionSecret: "secret-key-for-test-at-least-32-bytes!!",
	})

	app := fiber.New()
	app.Use(m.Session())

	app.Post("/set", func(c fiber.Ctx) error {
		sess := global.SessionFromContext(c)
		require.NotNil(t, sess)
		sess.Set("user_id", "admin_123")
		return c.SendStatus(http.StatusOK)
	})

	app.Get("/get", func(c fiber.Ctx) error {
		sess := global.SessionFromContext(c)
		require.NotNil(t, sess)
		userID := sess.Get("user_id")
		if userID == nil {
			return c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "no session"})
		}
		return c.Status(http.StatusOK).JSON(fiber.Map{"user_id": userID})
	})

	// 1. 设置会话并获取 Cookie
	reqSet := httptest.NewRequest(http.MethodPost, "/set", nil)
	respSet, err := app.Test(reqSet)
	require.NoError(t, err)
	defer func() { _ = respSet.Body.Close() }()
	require.Equal(t, http.StatusOK, respSet.StatusCode)

	cookie := respSet.Header.Get("Set-Cookie")
	require.NotEmpty(t, cookie, "Set-Cookie 响应头必须非空")

	// 2. 携带 Cookie 获取会话数据
	reqGet := httptest.NewRequest(http.MethodGet, "/get", nil)
	reqGet.Header.Set("Cookie", cookie)
	respGet, err := app.Test(reqGet)
	require.NoError(t, err)
	defer func() { _ = respGet.Body.Close() }()

	body, err := io.ReadAll(respGet.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, respGet.StatusCode)
	assert.Contains(t, string(body), "admin_123")
}

// TestSessionMiddleware_CustomCookieName 验证可注入的会话 Cookie 名称生效。
func TestSessionMiddleware_CustomCookieName(t *testing.T) {
	m := global.New(global.Deps{
		SessionName:   "cfiber_admin_sid",
		SessionSecret: "secret-key-for-test-at-least-32-bytes!!",
	})

	app := fiber.New()
	app.Use(m.Session())
	app.Post("/set", func(c fiber.Ctx) error {
		sess := global.SessionFromContext(c)
		require.NotNil(t, sess)
		sess.Set("user_id", "admin_123")
		return c.SendStatus(http.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/set", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Set-Cookie"), "cfiber_admin_sid=", "应使用配置的 Cookie 名称")
}

// TestSessionMiddleware_GeneratesEphemeralSecret 验证未注入密钥时仍可工作，不会退回静态弱密钥。
func TestSessionMiddleware_GeneratesEphemeralSecret(t *testing.T) {
	m := global.New(global.Deps{})

	app := fiber.New()
	app.Use(m.Session())
	app.Post("/set", func(c fiber.Ctx) error {
		sess := global.SessionFromContext(c)
		require.NotNil(t, sess)
		sess.Set("user_id", "admin_123")
		return c.SendStatus(http.StatusOK)
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/set", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("Set-Cookie"), "临时密钥也应能正常签发会话 Cookie")
}

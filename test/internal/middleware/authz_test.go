package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cfiber/internal/middleware"
	"cfiber/internal/middleware/global"
	"cfiber/pkg/authorization"
)

// TestAuthzMiddleware_RESTfulRules 验证基于角色、路径与 HTTP 方法执行 RESTful 访问控制。
func TestAuthzMiddleware_RESTfulRules(t *testing.T) {
	enforcer, err := authorization.New()
	require.NoError(t, err)

	mw := middleware.New(middleware.Deps{
		Authz: enforcer,
	})

	app := fiber.New(fiber.Config{
		ErrorHandler: global.HandleError,
	})

	adminGroup := app.Group("/v1/admin", mw.Authz())
	adminGroup.Get("/users", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "admin users list"})
	})

	clientGroup := app.Group("/v1", mw.Authz())
	clientGroup.Get("/users/me", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "client profile"})
	})

	// 1. admin 角色访问 /v1/admin/users：允许
	reqAdmin := httptest.NewRequest(http.MethodGet, "/v1/admin/users", nil)
	reqAdmin.SetBasicAuth("admin", "")
	respAdmin, err := app.Test(reqAdmin)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respAdmin.StatusCode)

	// 2. client 角色访问 /v1/admin/users：禁止（403）
	reqClientForbidden := httptest.NewRequest(http.MethodGet, "/v1/admin/users", nil)
	reqClientForbidden.SetBasicAuth("client", "")
	respClientForbidden, err := app.Test(reqClientForbidden)
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, respClientForbidden.StatusCode)

	// 3. client 角色访问 /v1/users/me：允许
	reqClientProfile := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	reqClientProfile.SetBasicAuth("client", "")
	respClientProfile, err := app.Test(reqClientProfile)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respClientProfile.StatusCode)

	// 4. 未认证用户（匿名）访问 /v1/admin/users：禁止（401 或 403）
	reqAnon := httptest.NewRequest(http.MethodGet, "/v1/admin/users", nil)
	respAnon, err := app.Test(reqAnon)
	require.NoError(t, err)
	assert.True(t, respAnon.StatusCode == http.StatusUnauthorized || respAnon.StatusCode == http.StatusForbidden)
}

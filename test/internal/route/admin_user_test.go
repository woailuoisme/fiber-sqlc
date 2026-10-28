// Package route_test 验证后台管理路由的注册与路由约束。
package route_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adminhandler "cfiber/internal/handler/admin"
	"cfiber/internal/middleware"
	adminroute "cfiber/internal/route/admin"
)

// TestRegisterAdminUserRoutes_Registration 验证路由正确挂载并携带 <guid> 约束。
func TestRegisterAdminUserRoutes_Registration(t *testing.T) {
	app := fiber.New()
	mw := middleware.New(middleware.Deps{})
	h := adminhandler.NewUserHandler(nil)

	adminroute.RegisterAdminUserRoutes(app, h, mw)

	routes := app.GetRoutes(true)
	var paths []string
	for _, r := range routes {
		paths = append(paths, r.Method+" "+r.Path)
	}

	assert.Contains(t, paths, "GET /admin/users/:id<guid>")
	assert.Contains(t, paths, "PATCH /admin/users/:id<guid>")
	assert.Contains(t, paths, "DELETE /admin/users/:id<guid>")
}

// TestAdminUserRoutes_GuidConstraint 验证 Fiber v3 原生 <guid> 约束在路由层即阻断非 UUID 请求。
func TestAdminUserRoutes_GuidConstraint(t *testing.T) {
	app := fiber.New()

	group := app.Group("/admin/users")
	group.Get("/:id<guid>", func(c fiber.Ctx) error {
		return c.SendString("matched:" + c.Params("id"))
	})

	// 非法 UUID（如纯数字或普通字符串）直接在路由匹配阶段返回 404，不进入处理器。
	invalidReq := httptest.NewRequest(http.MethodGet, "/admin/users/not-a-valid-uuid", nil)
	respInvalid, err := app.Test(invalidReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, respInvalid.StatusCode)

	// 合法格式的 UUID 能够命中路由并正确提取参数。
	validReq := httptest.NewRequest(http.MethodGet, "/admin/users/0192e2b3-847e-7c5b-9d47-68198f16182f", nil)
	respValid, err := app.Test(validReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, respValid.StatusCode)
	body, _ := io.ReadAll(respValid.Body)
	assert.Equal(t, "matched:0192e2b3-847e-7c5b-9d47-68198f16182f", string(body))
}

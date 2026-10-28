package middleware_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cfiber/internal/config"
	"cfiber/internal/database/sqlc"
	"cfiber/internal/enum"
	"cfiber/internal/middleware"
	"cfiber/internal/middleware/global"
	"cfiber/internal/user"
	"cfiber/pkg/authentication"
	"cfiber/pkg/authorization"
	"cfiber/pkg/cache"
	"cfiber/pkg/errs"
)

type memAuthBlacklistStore struct {
	mu   sync.RWMutex
	data map[string]string
}

func newMemAuthBlacklistStore() *memAuthBlacklistStore {
	return &memAuthBlacklistStore{data: make(map[string]string)}
}

func (s *memAuthBlacklistStore) Get(_ context.Context, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.data[key]
	if !ok {
		return "", cache.ErrNotFound
	}
	return val, nil
}

func (s *memAuthBlacklistStore) Set(_ context.Context, key, value string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

func (s *memAuthBlacklistStore) Exists(_ context.Context, key string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.data[key]
	return ok, nil
}

type mockUserRepo struct {
	users map[string]*sqlc.User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[string]*sqlc.User)}
}

func (r *mockUserRepo) FindByID(_ context.Context, id string) (*sqlc.User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	return u, nil
}

func (r *mockUserRepo) FindByCredentials(_ context.Context, creds authentication.Credentials) (*sqlc.User, error) {
	u, ok := r.users[creds.Account]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	return u, nil
}

func (r *mockUserRepo) ValidateCredentials(_ context.Context, _ *sqlc.User, _ authentication.Credentials) (bool, error) {
	return true, nil
}

// Identity 把测试用户投影为令牌身份。
func (r *mockUserRepo) Identity(u *sqlc.User) authentication.Identity {
	return user.Identity(u)
}

// setupAuthEnv 构建测试用中间件与守卫环境。
func setupAuthEnv() (
	*middleware.Middleware,
	*authentication.TokenIssuer,
	*authentication.Guard[*sqlc.User],
	*authentication.Guard[*sqlc.User],
	*mockUserRepo,
) {
	store := newMemAuthBlacklistStore()
	tokens := authentication.NewTokenIssuer(config.JWTConfig{
		Secret:     "0123456789abcdef0123456789abcdef",
		Issuer:     "cfiber-test",
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
	}, store)

	repo := newMockUserRepo()
	clientGuard := authentication.NewGuard(tokens, repo, user.ClientPolicy{}, repo.Identity)
	adminGuard := authentication.NewGuard(tokens, repo, user.AdminPolicy{}, repo.Identity)

	mgr := authentication.NewManager()
	mgr.Register("client", clientGuard)
	mgr.Register("admin", adminGuard)
	mgr.SetDefault("client")

	enforcer, err := authorization.New()
	if err != nil {
		panic(err)
	}

	m := middleware.New(middleware.Deps{
		AuthManager: mgr,
		Authz:       enforcer,
	})

	return m, tokens, clientGuard, adminGuard, repo
}

// TestAuth_MissingOrMalformedHeader 验证缺失或畸形 Authorization 报头时的 401 拦截。
func TestAuth_MissingOrMalformedHeader(t *testing.T) {
	m, _, _, _, _ := setupAuthEnv()

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Get("/protected", m.Auth(), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	testCases := []struct {
		header string
	}{
		{""},
		{"Basic dXNlcjpwYXNz"},
		{"Bearer "},
		{"Bearer"},
		{"Bearer token1 token2"},
	}

	for _, tc := range testCases {
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		if tc.header != "" {
			req.Header.Set("Authorization", tc.header)
		}
		resp, err := app.Test(req)
		require.NoError(t, err)

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var errResp errs.Response
		require.NoError(t, json.Unmarshal(body, &errResp))
		assert.Equal(t, string(errs.CodeAuthTokenInvalid), errResp.Code)
	}
}

// TestAuth_DefaultGuardSuccess 验证默认守卫 (client) 成功解析令牌并将身份写入上下文，同时支持多空格或制表符容错。
func TestAuth_DefaultGuardSuccess(t *testing.T) {
	m, _, clientGuard, _, repo := setupAuthEnv()

	repo.users["usr_client_1"] = &sqlc.User{
		ID:     "usr_client_1",
		Role:   enum.RoleUser,
		Status: enum.StatusActive,
		Name:   "Client User",
	}

	sess, err := clientGuard.Login(context.Background(), repo.users["usr_client_1"])
	require.NoError(t, err)

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})

	var capturedID authentication.Identity
	app.Get("/client/me", m.Auth(), func(c fiber.Ctx) error {
		id, ok := authentication.FromContext(c.Context())
		if ok {
			capturedID = id
		}
		return c.SendStatus(http.StatusOK)
	})

	// 验证标准单空格、连续多空格及制表符均可正确提取
	headers := []string{
		"Bearer " + sess.Token,
		"Bearer   " + sess.Token,
		"Bearer\t" + sess.Token,
	}

	for _, h := range headers {
		capturedID = authentication.Identity{}
		req := httptest.NewRequest(http.MethodGet, "/client/me", nil)
		req.Header.Set("Authorization", h)
		resp, err := app.Test(req)
		require.NoError(t, err)
		_ = resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "usr_client_1", capturedID.UserID)
		assert.Equal(t, string(enum.RoleUser), capturedID.Role)
	}
}

// TestAuth_NamedAdminGuard 验证指定 admin 守卫时的权限拦截与放行。
func TestAuth_NamedAdminGuard(t *testing.T) {
	m, _, clientGuard, adminGuard, repo := setupAuthEnv()

	repo.users["usr_normal"] = &sqlc.User{
		ID:     "usr_normal",
		Role:   enum.RoleUser,
		Status: enum.StatusActive,
	}
	repo.users["usr_admin"] = &sqlc.User{
		ID:     "usr_admin",
		Role:   enum.RoleAdmin,
		Status: enum.StatusActive,
	}

	clientSess, err := clientGuard.Login(context.Background(), repo.users["usr_normal"])
	require.NoError(t, err)

	adminSess, err := adminGuard.Login(context.Background(), repo.users["usr_admin"])
	require.NoError(t, err)

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Get("/admin/dashboard", m.Auth("admin"), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	// 1. 普通用户携带有效令牌访问 admin 接口，被 AdminPolicy 拦截返回 403 Forbidden
	req := httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+clientSess.Token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	var errResp errs.Response
	require.NoError(t, json.Unmarshal(body, &errResp))
	assert.Equal(t, string(errs.CodeForbidden), errResp.Code)

	// 2. 管理员携带有效令牌访问 admin 接口，放行 200 OK
	req = httptest.NewRequest(http.MethodGet, "/admin/dashboard", nil)
	req.Header.Set("Authorization", "Bearer "+adminSess.Token)
	resp, err = app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestAuth_RevocationEpoch 验证用户级注销纪元触发后，HTTP 网关即时拒绝已注销令牌。
func TestAuth_RevocationEpoch(t *testing.T) {
	m, _, clientGuard, _, repo := setupAuthEnv()

	repo.users["usr_revoked"] = &sqlc.User{
		ID:     "usr_revoked",
		Role:   enum.RoleUser,
		Status: enum.StatusActive,
	}

	sess, err := clientGuard.Login(context.Background(), repo.users["usr_revoked"])
	require.NoError(t, err)

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Get("/client/profile", m.Auth("client"), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	// 初始状态下有效
	req := httptest.NewRequest(http.MethodGet, "/client/profile", nil)
	req.Header.Set("Authorization", "Bearer "+sess.Token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 触发用户级全局作废
	err = clientGuard.InvalidateAll(context.Background(), "usr_revoked")
	require.NoError(t, err)

	// 再次请求返回 401 撤销错误
	req = httptest.NewRequest(http.MethodGet, "/client/profile", nil)
	req.Header.Set("Authorization", "Bearer "+sess.Token)
	resp, err = app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	var errResp errs.Response
	require.NoError(t, json.Unmarshal(body, &errResp))
	assert.Equal(t, string(errs.CodeAuthTokenRevoked), errResp.Code)
}

// TestAuth_NilManagerFailsClosed 验证未注入 AuthManager 时 fail-closed 返回 500。
func TestAuth_NilManagerFailsClosed(t *testing.T) {
	m := middleware.New(middleware.Deps{})

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Get("/protected", m.Auth(), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

// TestCan_Unauthenticated 验证无认证身份访问受保护资源时返回 401。
func TestCan_Unauthenticated(t *testing.T) {
	m, _, _, _, _ := setupAuthEnv()

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Get("/users", m.Can("users", "read"), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	var errResp errs.Response
	require.NoError(t, json.Unmarshal(body, &errResp))
	assert.Equal(t, string(errs.CodeAuthTokenInvalid), errResp.Code)
}

// TestCan_EnforcerNotConfigured 验证未配置 Authz Enforcer 时触发 500 内部错误。
func TestCan_EnforcerNotConfigured(t *testing.T) {
	m := middleware.New(middleware.Deps{
		Authz: nil,
	})

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Get("/users", func(c fiber.Ctx) error {
		ctx := authentication.With(c.Context(), authentication.Identity{
			UserID: "usr_admin_1",
			Role:   "admin",
		})
		c.SetContext(ctx)
		return c.Next()
	}, m.Can("users", "read"), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

// TestCan_RoleBasedAccess 验证基于 Casbin 角色策略的放行与拒绝。
func TestCan_RoleBasedAccess(t *testing.T) {
	m, _, _, _, _ := setupAuthEnv()

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Use(func(c fiber.Ctx) error {
		if role := c.Get("X-Test-Role"); role != "" {
			ctx := authentication.With(c.Context(), authentication.Identity{
				UserID: "usr_test",
				Role:   role,
			})
			c.SetContext(ctx)
		}
		return c.Next()
	})
	app.Get("/users", m.Can("users", "read"), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	// 1. 普通用户角色无权读取 users 资源 -> 403 Forbidden
	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("X-Test-Role", string(enum.RoleUser))
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	var errResp errs.Response
	require.NoError(t, json.Unmarshal(body, &errResp))
	assert.Equal(t, string(errs.CodeForbidden), errResp.Code)

	// 2. 管理员角色有权读取 users 资源 -> 200 OK
	req = httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("X-Test-Role", string(enum.RoleAdmin))
	resp, err = app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestAuth_MultiGuardFallback 验证多守卫回退匹配机制。
func TestAuth_MultiGuardFallback(t *testing.T) {
	m, _, clientGuard, _, repo := setupAuthEnv()

	repo.users["usr_user_1"] = &sqlc.User{
		ID:     "usr_user_1",
		Status: enum.StatusActive,
		Role:   enum.RoleUser,
	}

	clientSess, err := clientGuard.Login(context.Background(), repo.users["usr_user_1"])
	require.NoError(t, err)

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Get("/hybrid", m.Auth("admin", "client"), func(c fiber.Ctx) error {
		id, ok := authentication.FromContext(c.Context())
		if !ok {
			return c.SendStatus(http.StatusUnauthorized)
		}
		return c.Status(http.StatusOK).JSON(fiber.Map{"userId": id.UserID})
	})

	req := httptest.NewRequest(http.MethodGet, "/hybrid", nil)
	req.Header.Set("Authorization", "Bearer "+clientSess.Token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	var res map[string]string
	require.NoError(t, json.Unmarshal(body, &res))
	assert.Equal(t, "usr_user_1", res["userId"])
}

// TestCan_Permission 验证 m.Can 简写细粒度权限校验。
func TestCan_Permission(t *testing.T) {
	m, _, _, _, _ := setupAuthEnv()

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Use(func(c fiber.Ctx) error {
		if role := c.Get("X-Test-Role"); role != "" {
			ctx := authentication.With(c.Context(), authentication.Identity{
				UserID: "usr_admin_1",
				Role:   role,
			})
			c.SetContext(ctx)
		}
		return c.Next()
	})
	app.Get("/users", m.Can("users", "read"), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/users", nil)
	req.Header.Set("X-Test-Role", string(enum.RoleAdmin))
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

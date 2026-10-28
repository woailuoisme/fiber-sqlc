package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cfiber/internal/config"
	"cfiber/internal/middleware"
	"cfiber/internal/middleware/global"
	"cfiber/pkg/authentication"
	"cfiber/pkg/errs"
)

// mockRateLimitStore 模拟内存限流存储。
type mockRateLimitStore struct {
	mu   sync.Mutex
	data map[string][]byte
	err  error
}

func newMockRateLimitStore() *mockRateLimitStore {
	return &mockRateLimitStore{
		data: make(map[string][]byte),
	}
}

func (s *mockRateLimitStore) Get(key string) ([]byte, error) {
	return s.GetWithContext(context.Background(), key)
}

func (s *mockRateLimitStore) GetWithContext(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	val, ok := s.data[key]
	if !ok {
		return nil, nil
	}
	return val, nil
}

func (s *mockRateLimitStore) Set(key string, val []byte, exp time.Duration) error {
	return s.SetWithContext(context.Background(), key, val, exp)
}

func (s *mockRateLimitStore) SetWithContext(_ context.Context, key string, val []byte, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.data[key] = val
	return nil
}

func (s *mockRateLimitStore) Delete(key string) error {
	return s.DeleteWithContext(context.Background(), key)
}

func (s *mockRateLimitStore) DeleteWithContext(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func (s *mockRateLimitStore) Reset() error {
	return s.ResetWithContext(context.Background())
}

func (s *mockRateLimitStore) ResetWithContext(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string][]byte)
	return nil
}

func (s *mockRateLimitStore) Close() error {
	return nil
}

// TestRateLimit_Allowed 验证在配额内请求被正常放行并附带标准限流响应头。
func TestRateLimit_Allowed(t *testing.T) {
	store := newMockRateLimitStore()

	app := fiber.New()
	app.Use(middleware.ThrottleWithStore(store, 10, time.Minute, middleware.WithThrottleTier(middleware.TierAnonymous)))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "10", resp.Header.Get("X-RateLimit-Limit"))
	assert.Equal(t, "9", resp.Header.Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Reset"))
}

// TestRateLimit_Exceeded 验证超出配额时拦截为 429 并附带 Retry-After。
func TestRateLimit_Exceeded(t *testing.T) {
	store := newMockRateLimitStore()

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Use(middleware.ThrottleWithStore(store, 2, time.Minute, middleware.WithThrottleTier(middleware.TierAnonymous)))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	// 前 2 次允许
	for range 2 {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	// 第 3 次超限 (limit = 2)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, "2", resp.Header.Get("X-RateLimit-Limit"))
	assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Reset"))
	assert.NotEmpty(t, resp.Header.Get("Retry-After"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var res map[string]any
	require.NoError(t, json.Unmarshal(body, &res))
	assert.Equal(t, string(errs.CodeRateLimited), res["code"])
}

// TestRateLimit_SlidingWindow 验证滑动窗口达到配额后触发限流。
func TestRateLimit_SlidingWindow(t *testing.T) {
	store := newMockRateLimitStore()

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Use(middleware.ThrottleWithStore(store, 5, time.Minute, middleware.WithThrottleTier(middleware.TierAnonymous)))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	for range 5 {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	// 第 6 次触发 429
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

// TestRateLimit_StoreFailOpen 验证缓存/存储异常时降级放行。
func TestRateLimit_StoreFailOpen(t *testing.T) {
	store := newMockRateLimitStore()
	store.err = errors.New("redis connection refused")

	app := fiber.New()
	app.Use(middleware.ThrottleWithStore(store, 5, time.Minute, middleware.WithThrottleTier(middleware.TierAnonymous)))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// 存储故障时应当降级放行
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestRateLimit_NilStoreOrZeroLimit 验证存储为空或配额<=0时直接放行。
func TestRateLimit_NilStoreOrZeroLimit(t *testing.T) {
	// 1. nil store
	app1 := fiber.New()
	app1.Use(middleware.ThrottleWithStore(nil, 5, time.Minute, middleware.WithThrottleTier(middleware.TierAnonymous)))
	app1.Get("/test", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})
	resp1, err := app1.Test(httptest.NewRequest(http.MethodGet, "/test", nil))
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	require.Equal(t, http.StatusOK, resp1.StatusCode)

	// 2. 0 limit
	store := newMockRateLimitStore()
	app2 := fiber.New()
	app2.Use(middleware.ThrottleWithStore(store, 0, time.Minute, middleware.WithThrottleTier(middleware.TierAnonymous)))
	app2.Get("/test", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})
	resp2, err := app2.Test(httptest.NewRequest(http.MethodGet, "/test", nil))
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	require.Equal(t, http.StatusOK, resp2.StatusCode)
}

// TestRateLimit_AuthenticatedScope 验证认证用户维度按 UserID 隔离。
func TestRateLimit_AuthenticatedScope(t *testing.T) {
	store := newMockRateLimitStore()

	app := fiber.New()
	// 注入认证用户上下文
	app.Use(func(c fiber.Ctx) error {
		ctx := authentication.With(c.Context(), authentication.Identity{
			UserID: "usr_12345",
		})
		c.SetContext(ctx)
		return c.Next()
	})
	app.Use(middleware.ThrottleWithStore(store, 10, time.Minute, middleware.WithThrottleTier(middleware.TierAuthenticated)))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	// 验证存储中的 key 包含了 user:usr_12345
	assert.NotEmpty(t, store.data["ratelimit:authenticated:user:usr_12345"])
}

// TestThrottle_CustomWindow 验证支持任意时间窗口（如 10 秒）。
func TestThrottle_CustomWindow(t *testing.T) {
	store := newMockRateLimitStore()

	app := fiber.New()
	app.Use(middleware.ThrottleWithStore(store, 3, 10*time.Second))
	app.Get("/hourly", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/hourly", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "3", resp.Header.Get("X-RateLimit-Limit"))
	assert.Equal(t, "2", resp.Header.Get("X-RateLimit-Remaining"))
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Reset"))
}

// TestThrottle_CustomKey 验证支持自定义分区维度（如按登录账号 email 分区）。
func TestThrottle_CustomKey(t *testing.T) {
	store := newMockRateLimitStore()

	app := fiber.New()
	app.Use(middleware.ThrottleWithStore(
		store,
		5,
		time.Minute,
		middleware.WithThrottleKey(func(c fiber.Ctx) string {
			return "email:" + c.Query("email")
		}),
	))
	app.Get("/login-attempt", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/login-attempt?email=admin@example.com", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	expectedKey := "ratelimit:custom:email:admin@example.com"
	assert.NotEmpty(t, store.data[expectedKey])
}

// TestThrottle_Tier 验证通过 Middleware.Throttle 挂载特定业务档位（如 TierSensitive）。
func TestThrottle_Tier(t *testing.T) {
	store := newMockRateLimitStore()

	m := middleware.New(middleware.Deps{
		Limiter: config.LimiterConfig{
			Sensitive: 5,
		},
	})
	m.SetLimiterStore(store)

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Post("/auth/login", m.Throttle(middleware.TierSensitive), func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/auth/login", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Limit"))
}

// TestThrottle_ThrottleRate 验证通过 Middleware.ThrottleRate 按自定义限制与周期限流。
func TestThrottle_ThrottleRate(t *testing.T) {
	store := newMockRateLimitStore()

	m := middleware.New(middleware.Deps{})
	m.SetLimiterStore(store)

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	// 允许每 1 分钟 2 次请求
	app.Get("/api/fast", m.ThrottleRate(2, time.Minute), func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	// 第一次正常
	resp1, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/fast", nil))
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)
	assert.Equal(t, "2", resp1.Header.Get("X-RateLimit-Limit"))
	assert.Equal(t, "1", resp1.Header.Get("X-RateLimit-Remaining"))

	// 第二次正常
	resp2, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/fast", nil))
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	assert.Equal(t, "0", resp2.Header.Get("X-RateLimit-Remaining"))

	// 第三次触发限流 429
	resp3, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/fast", nil))
	require.NoError(t, err)
	defer func() { _ = resp3.Body.Close() }()
	assert.Equal(t, http.StatusTooManyRequests, resp3.StatusCode)
}

// TestThrottle_CustomResponse 验证自定义超限拦截响应回调。
func TestThrottle_CustomResponse(t *testing.T) {
	store := newMockRateLimitStore()

	m := middleware.New(middleware.Deps{})
	m.SetLimiterStore(store)

	customOpt := middleware.WithThrottleResponse(func(c fiber.Ctx) error {
		return c.Status(http.StatusTooManyRequests).JSON(fiber.Map{"custom_error": "please_wait"})
	})

	app := fiber.New()
	app.Get("/custom", m.ThrottleRate(1, time.Minute, customOpt), func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	resp1, err := app.Test(httptest.NewRequest(http.MethodGet, "/custom", nil))
	require.NoError(t, err)
	defer func() { _ = resp1.Body.Close() }()
	assert.Equal(t, http.StatusOK, resp1.StatusCode)

	resp2, err := app.Test(httptest.NewRequest(http.MethodGet, "/custom", nil))
	require.NoError(t, err)
	defer func() { _ = resp2.Body.Close() }()
	assert.Equal(t, http.StatusTooManyRequests, resp2.StatusCode)

	body, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	var res map[string]string
	require.NoError(t, json.Unmarshal(body, &res))
	assert.Equal(t, "please_wait", res["custom_error"])
}

// TestThrottle_DisabledGlobally 验证当 RATELIMIT_ENABLED=false 时，所有限流中间件完全旁路放行。
func TestThrottle_DisabledGlobally(t *testing.T) {
	store := newMockRateLimitStore()

	// 显式将限流总开关设为 false
	m := middleware.New(middleware.Deps{
		Limiter: config.LimiterConfig{
			Enabled:       new(bool),
			Sensitive:     1,
			Anonymous:     1,
			Authenticated: 1,
		},
	})
	m.SetLimiterStore(store)

	app := fiber.New(fiber.Config{
		ErrorHandler: global.New(global.Deps{}).ErrorHandler(),
	})
	app.Post("/auth/login", m.Throttle(middleware.TierSensitive), func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})
	app.Get("/rate", m.ThrottleRate(1, time.Minute), func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	// 连续发起 5 次敏感登录请求，均应直接放行，且不注入限流响应头
	for i := range 5 {
		resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/auth/login", nil))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "request %d should succeed", i+1)
		assert.Empty(t, resp.Header.Get("X-RateLimit-Limit"), "headers should not be set when rate limit is disabled")
	}

	// 连续发起 5 次自定义速率请求，同样全量放行
	for i := range 5 {
		resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/rate", nil))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		assert.Equal(t, http.StatusOK, resp.StatusCode, "request %d should succeed", i+1)
		assert.Empty(t, resp.Header.Get("X-RateLimit-Limit"), "headers should not be set when rate limit is disabled")
	}
}

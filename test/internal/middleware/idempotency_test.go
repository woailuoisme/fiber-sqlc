package middleware_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/idempotency"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cfiber/internal/middleware/global"
	"cfiber/pkg/errs"
)

// testIdempotencyKey 是符合默认 36 字符校验的幂等键。
const testIdempotencyKey = "019572d4-a6c8-78b1-8cf0-281b94c032a1"

// setupApp 装配错误信封与幂等中间件。
func setupApp(configs ...idempotency.Config) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: global.HandleError,
	})
	app.Use(global.NewIdempotency(nil, configs...))
	return app
}

type testResponse struct {
	Code   int
	Body   string
	Header http.Header
}

// doRequest 发起一次带幂等键的请求。
func doRequest(t *testing.T, app *fiber.App, method, path, key, body string, headers map[string]string) testResponse {
	t.Helper()

	var reader *bytes.Buffer
	if body != "" {
		reader = bytes.NewBufferString(body)
	} else {
		reader = bytes.NewBuffer(nil)
	}

	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	if key != "" {
		request.Header.Set(global.HeaderIdempotencyKey, key)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	resp, err := app.Test(request)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return testResponse{
		Code:   resp.StatusCode,
		Body:   string(bodyBytes),
		Header: resp.Header,
	}
}

// TestIdempotency_PassThroughWithoutKey 未携带幂等键时完全透传。
func TestIdempotency_PassThroughWithoutKey(t *testing.T) {
	app := setupApp()
	var calls atomic.Int32

	app.Post("/orders", func(c fiber.Ctx) error {
		calls.Add(1)
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "created"})
	})

	require.Equal(t, http.StatusOK, doRequest(t, app, http.MethodPost, "/orders", "", `{}`, nil).Code)
	require.Equal(t, http.StatusOK, doRequest(t, app, http.MethodPost, "/orders", "", `{}`, nil).Code)
	assert.EqualValues(t, 2, calls.Load())
}

// TestIdempotency_RejectsInvalidKey 默认校验要求 36 字符。
func TestIdempotency_RejectsInvalidKey(t *testing.T) {
	app := setupApp()
	app.Post("/orders", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "created"})
	})

	recorder := doRequest(t, app, http.MethodPost, "/orders", "short-key", `{}`, nil)
	require.True(t, recorder.Code == http.StatusBadRequest || recorder.Code == http.StatusUnprocessableEntity)
}

// TestIdempotency_ReplaysCompletedResponse 重复请求回放状态码、响应头与响应体。
func TestIdempotency_ReplaysCompletedResponse(t *testing.T) {
	app := setupApp()
	var calls atomic.Int32

	app.Post("/orders", func(c fiber.Ctx) error {
		calls.Add(1)
		c.Set("X-Business-Trace", "trace-001")
		return c.Status(http.StatusCreated).JSON(fiber.Map{"id": "ord_001"})
	})

	first := doRequest(t, app, http.MethodPost, "/orders", testIdempotencyKey, `{"item":"book"}`, nil)
	require.Equal(t, http.StatusCreated, first.Code)

	second := doRequest(t, app, http.MethodPost, "/orders", testIdempotencyKey, `{"item":"book"}`, nil)
	require.Equal(t, http.StatusCreated, second.Code)
	assert.JSONEq(t, first.Body, second.Body)
	assert.Equal(t, "trace-001", second.Header.Get("X-Business-Trace"))
	assert.EqualValues(t, 1, calls.Load())
}

// TestIdempotency_ReplaysEmptyBodyStatus 无响应体时仍需回放状态码。
func TestIdempotency_ReplaysEmptyBodyStatus(t *testing.T) {
	app := setupApp()
	var calls atomic.Int32

	app.Post("/no-content", func(c fiber.Ctx) error {
		calls.Add(1)
		return c.SendStatus(http.StatusNoContent)
	})

	require.Equal(t, http.StatusNoContent, doRequest(t, app, http.MethodPost, "/no-content", testIdempotencyKey, `{}`, nil).Code)
	require.Equal(t, http.StatusNoContent, doRequest(t, app, http.MethodPost, "/no-content", testIdempotencyKey, `{}`, nil).Code)
	assert.EqualValues(t, 1, calls.Load())
}

// TestIdempotency_ConcurrentRequestsExecuteOnce 相同键并发时只执行一次，其余等待后回放。
func TestIdempotency_ConcurrentRequestsExecuteOnce(t *testing.T) {
	app := setupApp()
	var calls atomic.Int32

	app.Post("/payments", func(c fiber.Ctx) error {
		calls.Add(1)
		time.Sleep(40 * time.Millisecond)
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "paid"})
	})

	const workers = 4
	var waitGroup sync.WaitGroup
	codes := make([]int, workers)
	reqErrors := make([]error, workers)
	waitGroup.Add(workers)
	for index := range workers {
		go func(i int) {
			defer waitGroup.Done()
			req := httptest.NewRequest(http.MethodPost, "/payments", bytes.NewBufferString(`{}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(global.HeaderIdempotencyKey, testIdempotencyKey)
			resp, err := app.Test(req)
			if err != nil {
				reqErrors[i] = err
				return
			}
			defer func() { _ = resp.Body.Close() }()
			codes[i] = resp.StatusCode
		}(index)
	}
	waitGroup.Wait()

	for _, err := range reqErrors {
		require.NoError(t, err)
	}
	for _, code := range codes {
		assert.Equal(t, http.StatusOK, code)
	}
	assert.EqualValues(t, 1, calls.Load())
}

// TestIdempotency_DoesNotCacheHandlerError 处理器返回错误时不缓存，允许重试。
func TestIdempotency_DoesNotCacheHandlerError(t *testing.T) {
	app := setupApp()
	var calls atomic.Int32

	app.Post("/flaky", func(c fiber.Ctx) error {
		if calls.Add(1) == 1 {
			return errs.Internal(errors.New("database unavailable"))
		}
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "recovered"})
	})

	first := doRequest(t, app, http.MethodPost, "/flaky", testIdempotencyKey, `{}`, nil)
	require.Equal(t, http.StatusInternalServerError, first.Code)

	second := doRequest(t, app, http.MethodPost, "/flaky", testIdempotencyKey, `{}`, nil)
	require.Equal(t, http.StatusOK, second.Code)
	assert.EqualValues(t, 2, calls.Load())
}

// TestIdempotency_KeepResponseHeaders 显式白名单只回放列出的响应头。
func TestIdempotency_KeepResponseHeaders(t *testing.T) {
	app := setupApp(idempotency.Config{KeepResponseHeaders: []string{"X-Keep"}})
	app.Post("/headers", func(c fiber.Ctx) error {
		c.Set("X-Keep", "kept")
		c.Set("X-Drop", "dropped")
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	require.Equal(t, http.StatusOK, doRequest(t, app, http.MethodPost, "/headers", testIdempotencyKey, `{}`, nil).Code)
	replay := doRequest(t, app, http.MethodPost, "/headers", testIdempotencyKey, `{}`, nil)

	assert.Equal(t, "kept", replay.Header.Get("X-Keep"))
	assert.Empty(t, replay.Header.Get("X-Drop"))
}

// TestIdempotency_LifetimeExpiry 缓存过期后重新执行处理器。
func TestIdempotency_LifetimeExpiry(t *testing.T) {
	app := setupApp(idempotency.Config{Lifetime: 1 * time.Second})
	var calls atomic.Int32

	app.Post("/expiring", func(c fiber.Ctx) error {
		calls.Add(1)
		return c.Status(http.StatusOK).JSON(fiber.Map{"call": calls.Load()})
	})

	require.Equal(t, http.StatusOK, doRequest(t, app, http.MethodPost, "/expiring", testIdempotencyKey, `{}`, nil).Code)
	time.Sleep(1100 * time.Millisecond)
	require.Equal(t, http.StatusOK, doRequest(t, app, http.MethodPost, "/expiring", testIdempotencyKey, `{}`, nil).Code)
	assert.EqualValues(t, 2, calls.Load())
}

// TestIdempotency_SkipsSafeMethods 默认不拦截无副作用的 HTTP 方法。
func TestIdempotency_SkipsSafeMethods(t *testing.T) {
	app := setupApp()
	var calls atomic.Int32

	app.Get("/status", func(c fiber.Ctx) error {
		calls.Add(1)
		return c.Status(http.StatusOK).JSON(fiber.Map{"alive": true})
	})

	require.Equal(t, http.StatusOK, doRequest(t, app, http.MethodGet, "/status", testIdempotencyKey, "", nil).Code)
	require.Equal(t, http.StatusOK, doRequest(t, app, http.MethodGet, "/status", testIdempotencyKey, "", nil).Code)
	assert.EqualValues(t, 2, calls.Load())
}

// TestIdempotency_PublicHelpers 暴露写入缓存与缓存回放状态。
func TestIdempotency_PublicHelpers(t *testing.T) {
	app := fiber.New(fiber.Config{
		ErrorHandler: global.HandleError,
	})

	var fromCache, putToCache atomic.Bool
	app.Use(func(c fiber.Ctx) error {
		err := c.Next()
		fromCache.Store(global.IsFromCache(c))
		putToCache.Store(global.WasPutToCache(c))
		return err
	})
	app.Use(global.NewIdempotency(nil))
	app.Post("/observe", func(c fiber.Ctx) error {
		return c.Status(http.StatusOK).JSON(fiber.Map{"status": "ok"})
	})

	require.Equal(t, http.StatusOK, doRequest(t, app, http.MethodPost, "/observe", testIdempotencyKey, `{}`, nil).Code)
	assert.False(t, fromCache.Load())
	assert.True(t, putToCache.Load())

	require.Equal(t, http.StatusOK, doRequest(t, app, http.MethodPost, "/observe", testIdempotencyKey, `{}`, nil).Code)
	assert.True(t, fromCache.Load())
	assert.False(t, putToCache.Load())
}

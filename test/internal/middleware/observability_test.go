package middleware_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cfiber/internal/middleware/global"
	"cfiber/pkg/errs"
	"cfiber/pkg/logger"
)

// TestAccessLog404SingleWarn 验证 404 请求仅输出一条 WARN 访问日志。
func TestAccessLog404SingleWarn(t *testing.T) {
	var buf bytes.Buffer
	testLogger := logger.NewWithOptions(
		logger.WithLevel("debug"),
		logger.WithChannel("json"),
		logger.WithWriter(&buf),
	)

	mw := global.New(global.Deps{Logger: testLogger})
	app := fiber.New(fiber.Config{
		ErrorHandler: mw.ErrorHandler(),
	})
	app.Use(
		mw.RequestID(),
		mw.AccessLog(),
	)

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/not-found", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 1, "404 请求必须且仅输出一条结构化访问日志，禁止重复打印告警日志")

	type logEntry struct {
		Level   string `json:"level"`
		Message string `json:"message"`
		Status  int    `json:"status"`
		Code    string `json:"code"`
		Error   string `json:"error"`
		Path    string `json:"path"`
		Method  string `json:"method"`
	}

	var entry logEntry
	unmarshalErr := json.Unmarshal([]byte(lines[0]), &entry)
	require.NoError(t, unmarshalErr)

	assert.Equal(t, "warn", entry.Level)
	assert.Contains(t, entry.Message, "access")
	assert.Equal(t, 404, entry.Status)
	assert.Equal(t, "NOT_FOUND", entry.Code)
	assert.Equal(t, "/not-found", entry.Path)
	assert.Equal(t, "GET", entry.Method)
}

// TestAccessLog500ErrorWithStack 验证 500 服务端内部错误具备调用栈日志与访问日志分工。
func TestAccessLog500ErrorWithStack(t *testing.T) {
	var buf bytes.Buffer
	testLogger := logger.NewWithOptions(
		logger.WithLevel("debug"),
		logger.WithChannel("json"),
		logger.WithWriter(&buf),
	)

	mw := global.New(global.Deps{Logger: testLogger})
	app := fiber.New(fiber.Config{
		ErrorHandler: mw.ErrorHandler(),
	})
	app.Use(
		mw.RequestID(),
		mw.AccessLog(),
	)

	app.Get("/panic-or-500", func(c fiber.Ctx) error {
		return errs.Internal(errors.New("db connection lost"))
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/panic-or-500", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 2, "500 请求应分别输出堆栈异常日志 (request processing failed) 与访问日志 (access)")

	type logEntry struct {
		Level   string `json:"level"`
		Message string `json:"message"`
		Status  int    `json:"status"`
		Stack   string `json:"stack"`
	}

	var errLog, accessLog logEntry
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &errLog))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &accessLog))

	assert.Equal(t, "error", errLog.Level)
	assert.Equal(t, "request processing failed", errLog.Message)
	assert.NotEmpty(t, errLog.Stack)

	assert.Equal(t, "error", accessLog.Level)
	assert.Contains(t, accessLog.Message, "access")
	assert.Equal(t, 500, accessLog.Status)
}

// TestAccessLog_ProbeSilencing 验证探针路径（/readyz 与 /livez）在正常状态（<400）下静默无日志，
// 但在发生异常（如 500）时依然正常输出日志。
func TestAccessLog_ProbeSilencing(t *testing.T) {
	var buf bytes.Buffer
	testLogger := logger.NewWithOptions(
		logger.WithLevel("debug"),
		logger.WithChannel("json"),
		logger.WithWriter(&buf),
	)

	mw := global.New(global.Deps{Logger: testLogger})
	app := fiber.New(fiber.Config{
		ErrorHandler: mw.ErrorHandler(),
	})
	app.Use(
		mw.RequestID(),
		mw.AccessLog(),
	)

	app.Get("/readyz", func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})
	app.Get("/livez", func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})
	app.Get("/readyz-fail", func(c fiber.Ctx) error {
		return errs.Internal(errors.New("db ping failed"))
	})

	// 1. 测试正常状态的 /readyz 与 /livez：不产生任何访问日志
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, strings.TrimSpace(buf.String()), "正常 /readyz 探测禁止输出访问日志")
	assert.Empty(t, resp.Header.Get("X-Request-ID"), "未提供追踪头的探针免于分配 UUID 追踪标识以优化 GC")

	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/livez", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, strings.TrimSpace(buf.String()), "正常 /livez 探测禁止输出访问日志")

	// 1.1 显式传递追踪头的探针请求必须被正常透传
	reqWithID := httptest.NewRequest(http.MethodGet, "/livez", nil)
	reqWithID.Header.Set("X-Request-ID", "probe-trace-123")
	respWithID, err := app.Test(reqWithID)
	require.NoError(t, err)
	defer func() { _ = respWithID.Body.Close() }()
	assert.Equal(t, "probe-trace-123", respWithID.Header.Get("X-Request-ID"))

	// 2. 测试异常状态的探针（状态码 >= 400 或 500）：必须输出告警或错误日志
	resp, err = app.Test(httptest.NewRequest(http.MethodGet, "/readyz-fail", nil))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.NotEmpty(t, strings.TrimSpace(buf.String()), "异常状态探针必须保留日志用于监控告警")
}

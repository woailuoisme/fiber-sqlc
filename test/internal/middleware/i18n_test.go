package middleware_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cfiber/internal/config"
	"cfiber/internal/middleware/global"
	"cfiber/pkg/i18n"
)

// TestI18nMiddleware_NegotiationAndMessages 验证多语言协商与上下文注入。
func TestI18nMiddleware_NegotiationAndMessages(t *testing.T) {
	translator, err := i18n.New(&config.Config{I18n: config.I18nConfig{DefaultLanguage: "en"}})
	require.NoError(t, err)

	m := global.New(global.Deps{
		Translator: translator,
	})

	app := fiber.New()
	app.Use(m.I18n())

	app.Get("/i18n-test", func(c fiber.Ctx) error {
		currentLang := global.LanguageFromContext(c)
		requiredMsg := translator.ValidationMessage(currentLang, "required", "email", "")
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"lang":     currentLang,
			"required": requiredMsg,
		})
	})

	// 1. 测试中文协商
	reqZH := httptest.NewRequest(http.MethodGet, "/i18n-test", nil)
	reqZH.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	respZH, err := app.Test(reqZH)
	require.NoError(t, err)
	defer func() { _ = respZH.Body.Close() }()

	bodyZH, _ := io.ReadAll(respZH.Body)
	require.Equal(t, http.StatusOK, respZH.StatusCode)
	assert.Contains(t, string(bodyZH), "zh-CN")

	// 2. 测试英文默认协商
	reqEN := httptest.NewRequest(http.MethodGet, "/i18n-test", nil)
	reqEN.Header.Set("Accept-Language", "en-US,en;q=0.9")
	respEN, err := app.Test(reqEN)
	require.NoError(t, err)
	defer func() { _ = respEN.Body.Close() }()

	bodyEN, _ := io.ReadAll(respEN.Body)
	require.Equal(t, http.StatusOK, respEN.StatusCode)
	assert.Contains(t, string(bodyEN), "en")
}

// TestI18nMiddleware_MatchesTranslatorOutput 锁定 HTTP 中间件与 pkg/i18n 的渲染结果一致。
func TestI18nMiddleware_MatchesTranslatorOutput(t *testing.T) {
	translator, err := i18n.New(&config.Config{I18n: config.I18nConfig{DefaultLanguage: "en"}})
	require.NoError(t, err)

	m := global.New(global.Deps{Translator: translator})

	app := fiber.New()
	app.Use(m.I18n())
	app.Get("/consistency", func(c fiber.Ctx) error {
		lang := global.LanguageFromContext(c)
		return c.Status(http.StatusOK).JSON(fiber.Map{
			"plain":     translator.Invoice(lang, "title", nil),
			"templated": translator.ValidationMessage(lang, "required", "name", ""),
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/consistency", nil)
	req.Header.Set("Accept-Language", "en")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var body struct {
		Plain     string `json:"plain"`
		Templated string `json:"templated"`
	}
	require.NoError(t, json.Unmarshal(bodyBytes, &body))

	assert.Equal(t, translator.Invoice("en", "title", nil), body.Plain)
	assert.Equal(t, translator.ValidationMessage("en", "required", "name", ""), body.Templated)
}

package global

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

// LanguageContextKey 是上下文中保存协商后语言标识的键。
const LanguageContextKey = "language"

// I18n 【全局中间件】统一提供多语言嗅探、语言协商与上下文注入。
func (m *Middleware) I18n() fiber.Handler {
	return func(c fiber.Ctx) error {
		acceptLanguage := strings.TrimSpace(c.Get(fiber.HeaderAcceptLanguage))
		lang := ""
		if m.translator != nil {
			lang = m.translator.Language(acceptLanguage)
		}
		if lang == "" {
			lang = "en"
		}

		fiber.Locals(c, LanguageContextKey, lang)
		return c.Next()
	}
}

// LanguageFromContext 从 Fiber 上下文提取已协商语言标识。
func LanguageFromContext(c fiber.Ctx) string {
	if c == nil {
		return ""
	}
	return fiber.Locals[string](c, LanguageContextKey)
}

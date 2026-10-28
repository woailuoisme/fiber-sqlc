package global

import (
	"errors"

	govalidator "github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"

	"cfiber/pkg/errs"
)

// ErrorHandler 返回可直接注入 fiber.Config.ErrorHandler 的错误处理函数。
func (m *Middleware) ErrorHandler() fiber.ErrorHandler {
	return m.HandleError
}

// HandleError 处理所有中间件、Handler 与 Panic 产生的错误，并渲染统一 JSON 错误信封。
func (m *Middleware) HandleError(c fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}

	if fields, ok := m.extractValidationFields(c, err); ok {
		err = errs.Validation(fields)
	}

	return errs.Render(c, err)
}

// HandleError 作为全局默认错误处理函数，供 fiber.Config.ErrorHandler 注入。
func HandleError(c fiber.Ctx, err error) error {
	defaultMw := New(Deps{})
	return defaultMw.HandleError(c, err)
}

// resolveLanguage 取当前请求语言。
func (m *Middleware) resolveLanguage(c fiber.Ctx) string {
	if lang := LanguageFromContext(c); lang != "" {
		return lang
	}

	acceptLang := c.Get(fiber.HeaderAcceptLanguage)
	if m.translator == nil {
		return acceptLang
	}
	return m.translator.Language(acceptLang)
}

// extractValidationFields 提取并翻译入参校验错误字典。
func (m *Middleware) extractValidationFields(c fiber.Ctx, err error) (map[string][]string, bool) {
	lang := m.resolveLanguage(c)

	if fields, ok := m.translateFieldKeys(lang, err); ok {
		return fields, true
	}

	if valErr, ok := errors.AsType[govalidator.ValidationErrors](err); ok {
		return m.translateValidation(lang, valErr), true
	}

	return nil, false
}

// translateValidation 渲染一条校验失败；未注入校验器时退回原始字段详情。
func (m *Middleware) translateValidation(lang string, valErr govalidator.ValidationErrors) map[string][]string {
	if m.validator != nil {
		return m.validator.Translate(m.translator, lang, valErr)
	}

	fields := make(map[string][]string, len(valErr))
	for _, fe := range valErr {
		field := errs.FieldPath(fe)
		fields[field] = append(fields[field], fe.Error())
	}
	return fields
}

// translateFieldKeys 将业务错误中基于键的字段明细渲染为指定语言文案。
func (m *Middleware) translateFieldKeys(lang string, err error) (map[string][]string, bool) {
	if m.translator == nil {
		return nil, false
	}

	bizErr, ok := errs.As(err)
	if !ok {
		return nil, false
	}

	keys := bizErr.FieldKeys()
	if len(keys) == 0 {
		return nil, false
	}

	fields := make(map[string][]string, len(keys))
	for field, messages := range keys {
		for _, message := range messages {
			rendered := m.translator.ValidationMessage(lang, message.Key, field, message.Param)
			if rendered != "" {
				fields[field] = append(fields[field], rendered)
			}
		}
	}
	return fields, len(fields) > 0
}

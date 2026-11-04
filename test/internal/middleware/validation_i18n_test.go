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
	appuser "cfiber/internal/user"
	"cfiber/pkg/errs"
	"cfiber/pkg/i18n"
)

// TestValidationMessageTranslation 验证以语言包键表达的字段级校验文案，
// 会在渲染边界按 Accept-Language 翻译，而不是把英文提示硬编码在业务代码里。
func TestValidationMessageTranslation(t *testing.T) {
	translator, err := i18n.New(config.I18nConfig{DefaultLanguage: "en"})
	require.NoError(t, err)

	cases := []struct {
		name     string
		language string
		want     string
	}{
		{name: "未指定语言时回退默认语言", want: "The name field cannot be null."},
		{name: "按 Accept-Language 渲染中文", language: "zh-CN", want: "名称不能为 null。"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := global.New(global.Deps{Translator: translator})
			app := fiber.New(fiber.Config{
				ErrorHandler: m.ErrorHandler(),
			})
			app.Patch("/profile", func(c fiber.Ctx) error {
				return appuser.FieldCannotBeNull("name")
			})

			req := httptest.NewRequest(http.MethodPatch, "/profile", nil)
			if tc.language != "" {
				req.Header.Set("Accept-Language", tc.language)
			}
			resp, err := app.Test(req)
			require.NoError(t, err)
			defer func() { _ = resp.Body.Close() }()

			bodyBytes, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, string(bodyBytes))

			var body errs.Response
			require.NoError(t, json.Unmarshal(bodyBytes, &body))
			assert.Equal(t, string(errs.CodeValidationFailed), body.Code)
			assert.Equal(t, []string{tc.want}, body.Errors["name"])
		})
	}
}

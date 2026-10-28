package global

import (
	"cmp"
	"strings"

	"github.com/gofiber/fiber/v3"

	"cfiber/pkg/errs"
)

// maxBodyBytes 非上传请求的请求体大小上限（1 MiB），与 api.md 的 413 契约保持一致。
const maxBodyBytes = 1 << 20

// defaultUploadBodyBytes multipart 上传请求的兜底上限（32 MiB）。
const defaultUploadBodyBytes = 32 << 20

// BodyLimit 限制请求体大小；multipart 上传按 UploadMaxBody 单独放行，否则媒体上传会被普通上限提前拦掉。
func (m *Middleware) BodyLimit() fiber.Handler {
	return func(c fiber.Ctx) error {
		body := c.Body()
		if len(body) == 0 {
			return c.Next()
		}

		limit := cmp.Or(m.maxBody, maxBodyBytes)
		if strings.HasPrefix(c.Get(fiber.HeaderContentType), fiber.MIMEMultipartForm) {
			limit = cmp.Or(m.uploadMaxBody, defaultUploadBodyBytes)
		}

		if int64(len(body)) > limit {
			return errs.New(errs.CodeRequestTooLarge, "Request body exceeds maximum allowed size")
		}

		return c.Next()
	}
}

// EnforceJSONContentType 要求 POST/PUT/PATCH 的实体载荷为 application/json。
//
// 旁路放行空载荷、文件/二进制上传与表单型 Webhook（如支付网关回调）；
// 分块传输仍进入校验，避免非标注入。
func (m *Middleware) EnforceJSONContentType() fiber.Handler {
	return func(c fiber.Ctx) error {
		method := c.Method()
		switch method {
		case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch:
			if len(c.Body()) == 0 {
				return c.Next()
			}

			contentType := c.Get(fiber.HeaderContentType)
			// 旁路放行：文件/媒体上传、二进制流、表单通知（如支付网关）
			if strings.HasPrefix(contentType, "multipart/") ||
				contentType == fiber.MIMEOctetStream ||
				contentType == fiber.MIMEApplicationForm {
				return c.Next()
			}

			if !strings.HasPrefix(contentType, fiber.MIMEApplicationJSON) {
				return errs.BadRequest("Content-Type must be application/json")
			}
		}

		return c.Next()
	}
}

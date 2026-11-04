package middleware

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"

	"cfiber/pkg/errs"
)

// Auth 解析并校验 Bearer 访问令牌，支持按序指定一个或多个命名守卫（如 "client"、"admin"）。
func (m *Middleware) Auth(guards ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if m.authManager == nil {
			return errs.Internal(errors.New("auth manager is not configured"))
		}

		raw, err := extractBearerToken(c.Get(fiber.HeaderAuthorization))
		if err != nil {
			return err
		}

		targetGuards := guards
		if len(targetGuards) == 0 {
			targetGuards = []string{""}
		}

		var lastErr error
		for _, name := range targetGuards {
			guard, err := m.authManager.Resolve(name)
			if err != nil {
				lastErr = errs.Internal(err)
				continue
			}

			reqCtx, authErr := guard.Authenticate(c.Context(), raw)
			if authErr == nil {
				c.SetContext(reqCtx)
				return c.Next()
			}
			lastErr = authErr
		}

		if lastErr != nil {
			return lastErr
		}
		return errs.Unauthorized(errs.CodeAuthTokenInvalid, "The token is invalid")
	}
}

// Can 校验已认证主体是否具备指定资源与动作的 RBAC 权限。
func (m *Middleware) Can(resource, action string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if m.authz == nil {
			return errs.Internal(errors.New("authorization enforcer is not configured"))
		}
		if err := m.authz.Authorize(c.Context(), resource, action); err != nil {
			return err
		}
		return c.Next()
	}
}

// extractBearerToken 从 Authorization 报头中提取 Bearer 令牌（RFC 6750 §2.1）。
func extractBearerToken(header string) (string, error) {
	fields := strings.Fields(header)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") || fields[1] == "" {
		return "", errs.Unauthorized(errs.CodeAuthTokenInvalid, "Missing or malformed Authorization header")
	}
	return fields[1], nil
}

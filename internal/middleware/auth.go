package middleware

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/gofiber/contrib/v3/casbin/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"

	"cfiber/pkg/authentication"
	"cfiber/pkg/errs"
)

var bearerExtractor = extractors.FromAuthHeader("Bearer")

// Auth 校验 Bearer 令牌并把认证身份注入上下文。
// 守卫可缺省（用默认守卫）、指定单个（"client"/"admin"）或按序回退（mw.Auth("admin", "client")）。
func (m *Middleware) Auth(guardNames ...string) fiber.Handler {
	guards := parseGuards(guardNames)

	return func(c fiber.Ctx) error {
		raw, err := bearerToken(c)
		if err != nil {
			return err
		}

		if m.authManager == nil {
			return errs.Internal(errors.New("middleware: auth manager is required"))
		}

		reqCtx, err := m.authenticateWithManager(c.Context(), raw, guards)
		if err != nil {
			return err
		}

		if m.authz != nil {
			reqCtx = authentication.WithGate(reqCtx, m.authz)
		}
		c.SetContext(reqCtx)

		return c.Next()
	}
}

// authenticateWithManager 基于注入的 AuthManager 解析守卫并验证令牌，支持多守卫顺序降级匹配。
func (m *Middleware) authenticateWithManager(ctx context.Context, raw string, guards []string) (context.Context, error) {
	if len(guards) == 0 {
		guard, err := m.authManager.Resolve()
		if err != nil {
			return nil, errs.Internal(err)
		}
		newCtx, _, authErr := guard.AuthenticateToken(ctx, raw)
		if authErr != nil {
			return nil, authErr
		}
		return newCtx, nil
	}

	var lastErr error
	for _, gName := range guards {
		guard, err := m.authManager.Resolve(gName)
		if err != nil {
			lastErr = errs.Internal(err)
			continue
		}

		newCtx, _, authErr := guard.AuthenticateToken(ctx, raw)
		if authErr == nil {
			return newCtx, nil
		}
		lastErr = authErr
	}

	if lastErr == nil {
		lastErr = errs.Unauthorized(errs.CodeAuthTokenInvalid, "Authentication is required")
	}
	return nil, lastErr
}

// Authz 【路由/组中间件】基于官方 gofiber/contrib/v3/casbin/v2 执行 Casbin RESTful 路径与 HTTP 方法访问控制。
func (m *Middleware) Authz() fiber.Handler {
	if m.authz == nil {
		return func(c fiber.Ctx) error { return c.Next() }
	}

	authorizer := casbin.New(casbin.Config{
		Enforcer: m.authz.CasbinEnforcer(),
		Lookup: func(c fiber.Ctx) string {
			if id, ok := authentication.FromContext(c.Context()); ok && id.Role != "" {
				return id.Role
			}
			if auth := c.Get(fiber.HeaderAuthorization); strings.HasPrefix(strings.ToLower(auth), "basic ") {
				if payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(auth[6:])); err == nil {
					parts := strings.SplitN(string(payload), ":", 2)
					if len(parts) > 0 && parts[0] != "" {
						return parts[0]
					}
				}
			}
			return ""
		},
		Unauthorized: func(c fiber.Ctx) error {
			return errs.Unauthorized(errs.CodeAuthTokenInvalid, "Authentication is required")
		},
		Forbidden: func(c fiber.Ctx) error {
			return errs.Forbidden("You are not allowed to perform this action")
		},
	})

	return authorizer.RoutePermission()
}

// Can 按资源+动作做 Casbin 资源级鉴权。
// 官方 authz 只支持 URL+Method，无法表达资源语义，故保留自建（路径级鉴权由 Authz 承担）。
func (m *Middleware) Can(resource, action string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if _, ok := authentication.FromContext(c.Context()); !ok {
			return errs.Unauthorized(errs.CodeAuthTokenInvalid, "Authentication is required")
		}

		if m.authz == nil {
			return errs.Internal(errors.New("middleware: authz enforcer not configured"))
		}

		if !m.authz.Can(c.Context(), resource, action) {
			return errs.Forbidden("You are not allowed to perform this action")
		}
		return c.Next()
	}
}

// parseGuards 解析可能存在的逗号分隔或变长守卫名称。
func parseGuards(guardNames []string) []string {
	guards := make([]string, 0, len(guardNames))
	for _, gn := range guardNames {
		for part := range strings.SplitSeq(gn, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				guards = append(guards, part)
			}
		}
	}
	return guards
}

// bearerToken 从 Authorization 头提取 Bearer 令牌，优先使用 Fiber RFC 兼容 Extractor。
func bearerToken(c fiber.Ctx) (string, error) {
	token, err := bearerExtractor.Extract(c)
	if err != nil || token == "" {
		header := strings.TrimSpace(c.Get(fiber.HeaderAuthorization))
		if header == "" {
			return "", errs.Unauthorized(errs.CodeAuthTokenInvalid, "Authentication is required")
		}
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return "", errs.Unauthorized(
				errs.CodeAuthTokenInvalid,
				"Authorization header must use the Bearer scheme",
			)
		}
		return parts[1], nil
	}
	return token, nil
}

// Package route 集中登记客户端 HTTP 端点：只做路由分组与中间件编排，不含业务逻辑。
package route

import (
	"github.com/gofiber/fiber/v3"

	"cfiber/internal/handler"
	"cfiber/internal/middleware"
)

// RegisterAuthRoutes 在 /v1/auth 下注册全部认证端点。
// 敏感端点走最严配额，需鉴权的端点走已认证配额，两者互不叠加。
func RegisterAuthRoutes(router fiber.Router, h *handler.AuthHandler, mw *middleware.Middleware) {
	group := router.Group("/auth")

	// 敏感端点：通过语义化子路由统一级联敏感配额限流中间件。
	sensitive := group.Group("", mw.Throttle(middleware.TierSensitive))
	sensitive.Post("/sign-up/email", h.SignUpEmail)
	sensitive.Post("/sign-in/email", h.SignInEmail)
	sensitive.Post("/sign-in/username", h.SignInUsername)
	sensitive.Post("/token/refresh", h.Refresh)
	sensitive.Post("/send-verification-otp", h.SendVerificationOTP)
	sensitive.Post("/verify-email", h.VerifyEmail)
	sensitive.Post("/forget-password", h.ForgetPassword)
	sensitive.Post("/request-password-reset", h.RequestPasswordReset)
	sensitive.Post("/reset-password", h.ResetPassword)

	// 需鉴权端点：通过语义化子路由统一级联已认证配额与身份校验中间件。
	protected := group.Group("", mw.Auth("client"), mw.Throttle(middleware.TierAuthenticated))
	protected.Get("/get-session", h.GetSession)
	protected.Post("/sign-out", h.SignOut)
	protected.Post("/change-password", h.ChangePassword)
}

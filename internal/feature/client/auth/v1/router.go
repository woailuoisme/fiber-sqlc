package v1

import (
	"github.com/gofiber/fiber/v3"

	"cfiber/internal/middleware"
)

// RegisterRoutes 在指定路由器下挂载认证端点 (/auth)。
func RegisterRoutes(router fiber.Router, h *Handler, mw *middleware.Middleware) {
	authGroup := router.Group("/auth")

	tierSensitive := mw.Throttle(middleware.TierSensitive)
	authGroup.Post("/sign-up/email", tierSensitive, h.SignUpEmail)
	authGroup.Post("/sign-in/email", tierSensitive, h.SignInEmail)
	authGroup.Post("/sign-in/username", tierSensitive, h.SignInUsername)
	authGroup.Post("/token/refresh", tierSensitive, h.Refresh)
	authGroup.Post("/send-verification-otp", tierSensitive, h.SendVerificationOTP)
	authGroup.Post("/verify-email", tierSensitive, h.VerifyEmail)
	authGroup.Post("/forget-password", tierSensitive, h.ForgetPassword)
	authGroup.Post("/request-password-reset", tierSensitive, h.RequestPasswordReset)
	authGroup.Post("/reset-password", tierSensitive, h.ResetPassword)

	clientAuth := mw.Auth("client")
	tierAuth := mw.Throttle(middleware.TierAuthenticated)
	authGroup.Get("/get-session", clientAuth, tierAuth, h.GetSession)
	authGroup.Post("/sign-out", clientAuth, tierAuth, h.SignOut)
	authGroup.Post("/change-password", clientAuth, tierAuth, h.ChangePassword)
}

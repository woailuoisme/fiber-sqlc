package bootstrap

import (
	"github.com/gofiber/fiber/v3"
	"github.com/samber/do/v2"

	"cfiber/internal/config"
	"cfiber/internal/database"
	"cfiber/internal/database/sqlc"
	"cfiber/internal/middleware"
	"cfiber/internal/middleware/global"
	"cfiber/internal/user"
	platformauth "cfiber/pkg/authentication"
	"cfiber/pkg/authorization"
	"cfiber/pkg/cache"
	"cfiber/pkg/hasher"
	"cfiber/pkg/i18n"
	"cfiber/pkg/logger"
	"cfiber/pkg/validator"
)

// AuthGuards 聚合客户端与管理端双重认证守卫。
type AuthGuards struct {
	Client *platformauth.Guard[*sqlc.User]
	Admin  *platformauth.Guard[*sqlc.User]
}

// SecurityPackage 声明认证令牌、用户领域服务、权限守卫与安全中间件。
var SecurityPackage = do.Package(
	// 1. JWT 双令牌签发器
	do.Lazy(func(i do.Injector) (*platformauth.TokenIssuer, error) {
		cfg := do.MustInvoke[*config.Config](i)
		c := do.MustInvoke[cache.Store](i)
		return platformauth.NewTokenIssuer(cfg.JWT, c), nil
	}),

	// 2. 用户领域内核服务（统一提供账号读写、本地口令凭据与 UserProvider 契约）
	do.Lazy(func(i do.Injector) (*user.Service, error) {
		db := do.MustInvoke[*database.DB](i)
		h := do.MustInvoke[hasher.PasswordHasher](i)
		return user.MustNewService(db, h), nil
	}),

	// 3. 双通道认证守卫（客户端与管理端）
	do.Lazy(func(i do.Injector) (AuthGuards, error) {
		cfg := do.MustInvoke[*config.Config](i)
		tokens := do.MustInvoke[*platformauth.TokenIssuer](i)
		userService := do.MustInvoke[*user.Service](i)

		clientGuard := platformauth.NewGuard(
			tokens,
			userService,
			user.ClientPolicy{RequireEmailVerification: cfg.Auth.RequireEmailVerification},
			userService.Identity,
		)
		adminGuard := platformauth.NewGuard(tokens, userService, user.AdminPolicy{}, userService.Identity)

		return AuthGuards{
			Client: clientGuard,
			Admin:  adminGuard,
		}, nil
	}),

	do.LazyNamed("client_guard", func(i do.Injector) (*platformauth.Guard[*sqlc.User], error) {
		guards := do.MustInvoke[AuthGuards](i)
		return guards.Client, nil
	}),

	do.LazyNamed("admin_guard", func(i do.Injector) (*platformauth.Guard[*sqlc.User], error) {
		guards := do.MustInvoke[AuthGuards](i)
		return guards.Admin, nil
	}),

	// 4. 认证管理器
	do.Lazy(func(i do.Injector) (*platformauth.Manager, error) {
		guards := do.MustInvoke[AuthGuards](i)
		return platformauth.NewManager().
			Register("client", guards.Client).
			Register("admin", guards.Admin).
			SetDefault("client"), nil
	}),

	// 5. 全局传输层中间件
	do.Lazy(func(i do.Injector) (*global.Middleware, error) {
		cfg := do.MustInvoke[*config.Config](i)
		c := do.MustInvoke[cache.Store](i)
		log := do.MustInvoke[*logger.Logger](i)
		translator := do.MustInvoke[i18n.Translator](i)
		val := do.MustInvoke[*validator.Validator](i)
		storage := do.MustInvoke[fiber.Storage](i)

		return global.New(global.Deps{
			Cache:          c,
			Storage:        storage,
			Logger:         log,
			CORS:           cfg.CORS,
			Production:     cfg.IsProduction(),
			Translator:     translator,
			Validator:      val,
			RequestTimeout: cfg.App.Server.RequestTimeout,
			UploadMaxBody:  cfg.Media.MaxFileSize,
		}), nil
	}),

	// 6. 路由级传输层中间件
	do.Lazy(func(i do.Injector) (*middleware.Middleware, error) {
		cfg := do.MustInvoke[*config.Config](i)
		authManager := do.MustInvoke[*platformauth.Manager](i)
		storage := do.MustInvoke[fiber.Storage](i)
		enforcer := do.MustInvoke[*authorization.Enforcer](i)
		log := do.MustInvoke[*logger.Logger](i)

		return middleware.New(middleware.Deps{
			AuthManager: authManager,
			Storage:     storage,
			Authz:       enforcer,
			Logger:      log,
			Limiter:     cfg.Limiter,
		}), nil
	}),
)

package admin

import (
	"github.com/samber/do/v2"

	"cfiber/internal/database/sqlc"
	adminauth "cfiber/internal/feature/admin/auth"
	adminuser "cfiber/internal/feature/admin/user"
	"cfiber/internal/user"
	platformauth "cfiber/pkg/authentication"
	"cfiber/pkg/media"
)

// Package 声明 admin 业务特性的 DI 依赖定义。
var Package = do.Package(
	// 1. 管理端认证服务与 HTTP Handler
	do.Lazy(func(i do.Injector) (*adminauth.AuthService, error) {
		guard := do.MustInvokeNamed[*platformauth.Guard[*sqlc.User]](i, "admin_guard")
		userService := do.MustInvoke[*user.Service](i)
		return adminauth.MustNewAuthService(adminauth.AuthDeps{
			Guard:     guard,
			Passwords: userService,
		}), nil
	}),

	do.Lazy(func(i do.Injector) (*adminauth.Handler, error) {
		svc := do.MustInvoke[*adminauth.AuthService](i)
		return adminauth.NewHandler(svc), nil
	}),

	// 2. 管理端用户服务与 HTTP Handler
	do.Lazy(func(i do.Injector) (*adminuser.UserService, error) {
		userService := do.MustInvoke[*user.Service](i)
		signer := do.MustInvoke[media.Signer](i)
		return adminuser.MustNewUserService(adminuser.UserDeps{
			UserService: userService,
			Signer:      signer,
		}), nil
	}),

	do.Lazy(func(i do.Injector) (*adminuser.Handler, error) {
		svc := do.MustInvoke[*adminuser.UserService](i)
		return adminuser.NewHandler(svc), nil
	}),

	// 3. 聚合 Handlers 供路由层统一装配
	do.Lazy(func(i do.Injector) (Handlers, error) {
		return Handlers{
			Auth: do.MustInvoke[*adminauth.Handler](i),
			User: do.MustInvoke[*adminuser.Handler](i),
		}, nil
	}),
)

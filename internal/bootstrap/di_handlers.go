package bootstrap

import (
	"github.com/gofiber/fiber/v3"
	"github.com/samber/do/v2"

	"cfiber/internal/config"
	"cfiber/internal/feature/admin"
	"cfiber/internal/feature/client"
	"cfiber/internal/feature/system"
	"cfiber/internal/middleware"
	"cfiber/internal/middleware/global"
	"cfiber/pkg/validator"
)

// HandlersPackage 声明 HTTP 传输适配层（路由处理器聚合与 Fiber 引擎）。
var HandlersPackage = do.Package(
	// 1. 路由处理器聚合
	do.Lazy(func(i do.Injector) (routeHandlers, error) {
		return routeHandlers{
			system: do.MustInvoke[*system.SystemHandler](i),
			client: do.MustInvoke[client.Handlers](i),
			admin:  do.MustInvoke[admin.Handlers](i),
		}, nil
	}),

	// 2. Fiber HTTP 引擎与洋葱中间件管线
	do.Lazy(func(i do.Injector) (*fiber.App, error) {
		cfg := do.MustInvoke[*config.Config](i)
		globalMW := do.MustInvoke[*global.Middleware](i)
		routeMW := do.MustInvoke[*middleware.Middleware](i)
		handlers := do.MustInvoke[routeHandlers](i)
		val := do.MustInvoke[*validator.Validator](i)

		return buildEngine(cfg, globalMW, routeMW, handlers, val)
	}),
)

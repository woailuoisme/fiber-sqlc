package bootstrap

import (
	"errors"

	"github.com/gofiber/fiber/v3"

	"cfiber/internal/config"
	"cfiber/internal/handler"
	adminhandler "cfiber/internal/handler/admin"
	"cfiber/internal/middleware"
	"cfiber/internal/middleware/global"
	"cfiber/internal/route"
	adminroute "cfiber/internal/route/admin"
	"cfiber/pkg/validator"
)

// routeHandlers 汇总各模块的 HTTP 处理器，由装配方填充。
type routeHandlers struct {
	system    *handler.SystemHandler
	auth      *handler.AuthHandler
	profile   *handler.ProfileHandler
	admin     *adminhandler.UserHandler
	adminAuth *adminhandler.AuthHandler
	realtime  *handler.RealtimeHandler
	payment   *handler.PaymentHandler
}

// mount 把全部模块挂载到 /v1 分组下。
func (h routeHandlers) mount(v1 fiber.Router, mw *middleware.Middleware) {
	route.RegisterAuthRoutes(v1, h.auth, mw)
	route.RegisterProfileRoutes(v1, h.profile, mw)
	adminroute.RegisterAdminAuthRoutes(v1, h.adminAuth, mw)
	adminroute.RegisterAdminUserRoutes(v1, h.admin, mw)
	route.RegisterRealtimeRoutes(v1, h.realtime, mw)
	route.RegisterPaymentRoutes(v1, h.payment, mw)
}

// zeroHandlers 提供仅用于撑起路由表的零值处理器。
func zeroHandlers() routeHandlers {
	return routeHandlers{
		system:    &handler.SystemHandler{},
		auth:      &handler.AuthHandler{},
		profile:   &handler.ProfileHandler{},
		admin:     &adminhandler.UserHandler{},
		adminAuth: &adminhandler.AuthHandler{},
		realtime:  &handler.RealtimeHandler{},
		payment:   &handler.PaymentHandler{},
	}
}

// buildEngine 组装 fiber 引擎与全部路由。
func buildEngine(
	cfg *config.Config,
	globalMW *global.Middleware,
	routeMW *middleware.Middleware,
	handlers routeHandlers,
	val *validator.Validator,
) (*fiber.App, error) {
	app := fiber.New(fiber.Config{
		AppName:         cfg.App.Name,
		ServerHeader:    "cfiber",
		ErrorHandler:    globalMW.HandleError,
		StructValidator: val,
		BodyLimit:       32 * 1024 * 1024,
		ReadTimeout:     cfg.App.Server.ReadTimeout,
		WriteTimeout:    cfg.App.Server.WriteTimeout,
		IdleTimeout:     cfg.App.Server.IdleTimeout,
		// 维持默认极速零内存分配模式（Immutable: false）。
		// 跨协程或逃逸请求生命周期的 Header/参数必须通过 utils.CopyString 复制独立副本。
		Immutable:  false,
		TrustProxy: true,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Loopback: true,
			Private:  true,
		},
	})

	// 全局中间件链按洋葱模型分层，顺序即依赖：
	app.Use(
		// 静态资源与浏览器降噪
		globalMW.Favicon(),

		// 链路观测
		globalMW.Telemetry(),
		globalMW.RequestID(),
		globalMW.AccessLog(),

		// 协议安全
		globalMW.CORS(),
		globalMW.Session(),
		globalMW.I18n(),

		// 异常治理与超时
		globalMW.Recovery(),
		globalMW.Timeout(),

		// 载荷控制
		globalMW.BodyLimit(),
		globalMW.EnforceJSONContentType(),
		globalMW.Idempotency(),
	)

	// 注册根路径与系统探针路由（/、/livez、/readyz）
	route.RegisterSystemRoutes(app, handlers.system)

	// 仅在开启 ENABLE_DOCS 时注册 API 文档路由
	if cfg.App.EnableDocs {
		registerDocs(app)
	}

	v1 := app.Group("/v1")
	handlers.mount(v1, routeMW)

	return app, nil
}

// InspectEngine 组装用于路由探测与文档自省的轻量级 Fiber 引擎实例（零数据库与缓存网络连接）。
func InspectEngine(cfg *config.Config) (*fiber.App, error) {
	if cfg == nil {
		return nil, errors.New("bootstrap: inspect engine requires a non-nil config")
	}

	val, _ := validator.New()

	globalMW := global.New(global.Deps{
		CORS:           cfg.CORS,
		Production:     cfg.IsProduction(),
		RequestTimeout: cfg.App.Server.RequestTimeout,
		Validator:      val,
	})

	routeMW := middleware.New(middleware.Deps{
		Limiter: cfg.Limiter,
	})

	return buildEngine(cfg, globalMW, routeMW, zeroHandlers(), val)
}

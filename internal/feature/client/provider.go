package client

import (
	"github.com/samber/do/v2"

	"cfiber/internal/config"
	"cfiber/internal/database"
	"cfiber/internal/database/sqlc"
	clientauth "cfiber/internal/feature/client/auth"
	clientmedia "cfiber/internal/feature/client/media"
	clientpayment "cfiber/internal/feature/client/payment"
	clientprofile "cfiber/internal/feature/client/profile"
	clientrealtime "cfiber/internal/feature/client/realtime"
	"cfiber/internal/user"
	platformauth "cfiber/pkg/authentication"
	"cfiber/pkg/cache"
	"cfiber/pkg/eventbus"
	"cfiber/pkg/i18n"
	"cfiber/pkg/mailer"
	"cfiber/pkg/media"
	"cfiber/pkg/notification"
	"cfiber/pkg/payment"
	platformRealtime "cfiber/pkg/realtime"
	"cfiber/pkg/storage"
)

// Package 声明 client 业务特性的 DI 依赖定义。
var Package = do.Package(
	// 1. 客户端认证服务与 HTTP Handler
	do.Lazy(func(i do.Injector) (*clientauth.AuthService, error) {
		db := do.MustInvoke[*database.DB](i)
		userService := do.MustInvoke[*user.Service](i)
		guard := do.MustInvokeNamed[*platformauth.Guard[*sqlc.User]](i, "client_guard")
		c := do.MustInvoke[cache.Store](i)
		mailerClient := do.MustInvoke[mailer.Mailer](i)
		notifier := do.MustInvoke[*notification.Notifier](i)
		translator := do.MustInvoke[i18n.Translator](i)
		cfg := do.MustInvoke[*config.Config](i)
		pub := do.MustInvoke[eventbus.Publisher](i)

		return clientauth.MustNewAuthService(clientauth.AuthDeps{
			DB:         db,
			Users:      userService,
			Guard:      guard,
			Cache:      c,
			Mailer:     mailerClient,
			Notifier:   notifier,
			Translator: translator,
			Config:     cfg,
			Publisher:  pub,
		}), nil
	}),

	do.Lazy(func(i do.Injector) (*clientauth.Handler, error) {
		svc := do.MustInvoke[*clientauth.AuthService](i)
		return clientauth.NewHandler(svc), nil
	}),

	// 2. 个人档案服务与 HTTP Handler
	do.Lazy(func(i do.Injector) (*clientprofile.ProfileService, error) {
		userService := do.MustInvoke[*user.Service](i)
		disk := do.MustInvoke[storage.Disk](i)
		return clientprofile.MustNewProfileService(clientprofile.ProfileDeps{
			Users:   userService,
			Storage: disk,
		}), nil
	}),

	do.Lazy(func(i do.Injector) (*clientprofile.Handler, error) {
		svc := do.MustInvoke[*clientprofile.ProfileService](i)
		return clientprofile.NewHandler(svc), nil
	}),

	// 3. 支付服务与 HTTP Handler
	do.Lazy(func(i do.Injector) (*clientpayment.PaymentService, error) {
		gateway := do.MustInvoke[payment.Gateway](i)
		c := do.MustInvoke[cache.Store](i)
		cfg := do.MustInvoke[*config.Config](i)
		pub := do.MustInvoke[eventbus.Publisher](i)

		return clientpayment.NewPaymentService(gateway, c, cfg, pub), nil
	}),

	do.Lazy(func(i do.Injector) (*clientpayment.Handler, error) {
		svc := do.MustInvoke[*clientpayment.PaymentService](i)
		return clientpayment.NewHandler(svc), nil
	}),

	// 4. 实时通信服务与 HTTP Handler
	do.Lazy(func(i do.Injector) (*clientrealtime.RealtimeService, error) {
		gateway := do.MustInvoke[platformRealtime.Gateway](i)
		cfg := do.MustInvoke[*config.Config](i)
		userService := do.MustInvoke[*user.Service](i)

		return clientrealtime.NewRealtimeService(clientrealtime.RealtimeDeps{
			Gateway: gateway,
			Cfg:     cfg,
			Secret:  cfg.Realtime.Secret,
			Users:   userService,
		}), nil
	}),

	do.Lazy(func(i do.Injector) (*clientrealtime.Handler, error) {
		svc := do.MustInvoke[*clientrealtime.RealtimeService](i)
		return clientrealtime.NewHandler(svc), nil
	}),

	// 5. 媒体处理 HTTP Handler
	do.Lazy(func(i do.Injector) (*clientmedia.Handler, error) {
		uploader := do.MustInvoke[media.Uploader](i)
		return clientmedia.NewHandler(uploader), nil
	}),

	// 6. 聚合 Handlers 供路由层统一装配
	do.Lazy(func(i do.Injector) (Handlers, error) {
		return Handlers{
			Auth:     do.MustInvoke[*clientauth.Handler](i),
			Profile:  do.MustInvoke[*clientprofile.Handler](i),
			Payment:  do.MustInvoke[*clientpayment.Handler](i),
			Media:    do.MustInvoke[*clientmedia.Handler](i),
			Realtime: do.MustInvoke[*clientrealtime.Handler](i),
		}, nil
	}),
)

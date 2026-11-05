package system

import (
	"context"

	"github.com/samber/do/v2"

	"cfiber/internal/database"
	"cfiber/pkg/cache"
	"cfiber/pkg/logger"
	"cfiber/pkg/mailer"
	"cfiber/pkg/media"
	platformRealtime "cfiber/pkg/realtime"
	"cfiber/pkg/search"
	"cfiber/pkg/storage"
)

type databasePinger struct{ db *database.DB }

func (p *databasePinger) Ping(ctx context.Context) error { return p.db.Ping(ctx) }
func (*databasePinger) Driver() string                   { return "postgres" }

// Package 声明 system 特性包的 DI 依赖定义。
var Package = do.Package(
	do.Lazy(func(i do.Injector) (*SystemService, error) {
		db := do.MustInvoke[*database.DB](i)
		c := do.MustInvoke[cache.Store](i)
		disk := do.MustInvoke[storage.Disk](i)
		searchClient := do.MustInvoke[search.Client](i)
		gateway := do.MustInvoke[platformRealtime.Gateway](i)
		signer := do.MustInvoke[media.Signer](i)
		mailerClient := do.MustInvoke[mailer.Mailer](i)

		return NewSystemService(SystemDeps{
			Database: &databasePinger{db: db},
			Cache:    c,
			Storage:  disk,
			Search:   searchClient,
			Realtime: gateway,
			Imgproxy: signer,
			Mailer:   mailerClient,
		}), nil
	}),

	do.Lazy(func(i do.Injector) (*SystemHandler, error) {
		svc := do.MustInvoke[*SystemService](i)
		log := do.MustInvoke[*logger.Logger](i)
		return NewSystemHandler(svc, log), nil
	}),
)

package bootstrap

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/sync/errgroup"

	"cfiber/pkg/logger"
)

// Serve 启动 Fiber HTTP 服务与后台路由器，阻塞直到平滑停机。
//
// 全部长生命周期 goroutine 由同一个 errgroup 托管：任一环节异常即取消其余环节，
// Wait 返回后才释放数据库与缓存连接，停机顺序确定。
func (a *Application) Serve(ctx context.Context) error {
	srvCfg := a.cfg.App.Server
	shutdownTimeout := cmp.Or(srvCfg.ShutdownTimeout, 3*time.Second)

	a.engine.Hooks().OnListen(func(data fiber.ListenData) error {
		if fiber.IsChild() {
			return nil
		}
		a.logStartup(data)
		return nil
	})

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		addr := ":" + strconv.Itoa(a.cfg.App.Port)

		listenConfig := fiber.ListenConfig{
			DisableStartupMessage: true,
			GracefulContext:       groupCtx,
			ShutdownTimeout:       shutdownTimeout,
		}

		if err := a.engine.Listen(addr, listenConfig); err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("fiber server exited unexpectedly: %w", err)
		}
		return nil
	})

	// 事件路由器与 HTTP 同生命周期，随上下文一起退出。
	if a.eventbus != nil {
		group.Go(func() error {
			if err := a.eventbus.Run(groupCtx); err != nil && !errors.Is(err, context.Canceled) {
				return fmt.Errorf("event bus router stopped: %w", err)
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

// logStartup 输出启动横幅，从 OnListen 钩子中读取实际绑定的主机、端口与 TLS 状态。
func (a *Application) logStartup(data fiber.ListenData) {
	scheme := "http"
	if data.TLS {
		scheme = "https"
	}
	host := data.Host
	if host == "0.0.0.0" || host == "" || host == "[::1]" || host == "::" {
		host = "localhost"
	}
	base := fmt.Sprintf("%s://%s:%s", scheme, host, data.Port)
	addr := net.JoinHostPort(data.Host, data.Port)

	log := a.logger
	if log == nil {
		log = logger.Default()
	}

	attrs := []slog.Attr{
		slog.String("addr", addr),
		slog.String("url", base),
		slog.String("env", a.cfg.App.Env),
		slog.String("storage", a.cfg.Storage.Driver),
	}
	if a.cfg.App.EnableDocs {
		attrs = append(attrs, slog.String("docs", base+docsPagePath))
	}
	log.LogAttrs(context.Background(), slog.LevelInfo, "HTTP server started and listening", attrs...)
}

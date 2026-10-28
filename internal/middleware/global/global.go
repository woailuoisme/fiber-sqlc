// Package global 提供全局 HTTP 传输层中间件，挂载顺序见 internal/bootstrap/engine.go。
//
// 官方 Fiber 生态能力覆盖：
// Favicon / Telemetry / CORS / Timeout / RequestID / AccessLog / Recover / Idempotency。
package global

import (
	"cmp"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/rs/zerolog"

	"cfiber/internal/config"
	"cfiber/pkg/cache"
	"cfiber/pkg/i18n"
	"cfiber/pkg/logger"
	"cfiber/pkg/validator"
)

// Deps 描述全局中间件所需的全部外部依赖。
type Deps struct {
	Cache          cache.Store
	Storage        fiber.Storage
	Logger         *zerolog.Logger
	CORS           config.CORSConfig
	Production     bool
	Translator     i18n.Translator
	Validator      *validator.Validator
	RequestTimeout time.Duration
	// MaxBody 非 multipart 请求的请求体上限（字节，0 表示用默认值）。
	MaxBody int64
	// UploadMaxBody multipart/form-data 上传请求的请求体上限（字节，0 表示用默认值）。
	UploadMaxBody int64
}

// Middleware 承载全局传输层中间件的实例依赖。
type Middleware struct {
	cache          cache.Store
	storage        fiber.Storage
	logger         *zerolog.Logger
	cors           config.CORSConfig
	production     bool
	translator     i18n.Translator
	validator      *validator.Validator
	requestTimeout time.Duration
	maxBody        int64
	uploadMaxBody  int64
}

// New 构造全局中间件实例。
//
// 未注入 Logger 时回退到 logger.Nop()，使得各中间件可以无条件链式打日志。
func New(deps Deps) *Middleware {
	return &Middleware{
		cache:          deps.Cache,
		storage:        deps.Storage,
		logger:         cmp.Or(deps.Logger, logger.Nop()),
		cors:           deps.CORS,
		production:     deps.Production,
		translator:     deps.Translator,
		validator:      deps.Validator,
		requestTimeout: deps.RequestTimeout,
		maxBody:        deps.MaxBody,
		uploadMaxBody:  deps.UploadMaxBody,
	}
}

// SetStorage 允许注入或替换底层 Fiber Storage 驱动。
func (m *Middleware) SetStorage(s fiber.Storage) {
	m.storage = s
}

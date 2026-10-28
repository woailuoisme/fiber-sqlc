package global

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/idempotency"
)

// HeaderIdempotencyKey 是 IETF 标准幂等键请求头。
const HeaderIdempotencyKey = "Idempotency-Key"

// Idempotency 构造全局挂载的请求幂等中间件，基于 fiber/v3/middleware/idempotency。
func (m *Middleware) Idempotency() fiber.Handler {
	return NewIdempotency(m.storage)
}

// NewIdempotency 构造独立幂等中间件实例，方便测试与存储注入。
func NewIdempotency(storage fiber.Storage, configs ...idempotency.Config) fiber.Handler {
	cfg := idempotency.Config{
		KeyHeader: HeaderIdempotencyKey,
		Storage:   storage,
		Lifetime:  30 * time.Minute,
	}
	if len(configs) > 0 {
		cfg = mergeIdempotencyConfig(cfg, configs[0])
	}
	return idempotency.New(cfg)
}

func mergeIdempotencyConfig(base, custom idempotency.Config) idempotency.Config {
	if custom.KeyHeader != "" {
		base.KeyHeader = custom.KeyHeader
	}
	if custom.Storage != nil {
		base.Storage = custom.Storage
	}
	if custom.Lifetime > 0 {
		base.Lifetime = custom.Lifetime
	}
	if custom.KeyHeaderValidate != nil {
		base.KeyHeaderValidate = custom.KeyHeaderValidate
	}
	if len(custom.KeepResponseHeaders) > 0 {
		base.KeepResponseHeaders = custom.KeepResponseHeaders
	}
	if custom.Lock != nil {
		base.Lock = custom.Lock
	}
	if custom.Next != nil {
		base.Next = custom.Next
	}
	return base
}

// IsFromCache 判定当前请求是否命中幂等缓存。
func IsFromCache(c fiber.Ctx) bool {
	return idempotency.IsFromCache(c)
}

// WasPutToCache 判定当前请求是否被写入幂等缓存。
func WasPutToCache(c fiber.Ctx) bool {
	return idempotency.WasPutToCache(c)
}

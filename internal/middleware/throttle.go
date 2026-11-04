package middleware

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"

	"cfiber/pkg/authentication"
	"cfiber/pkg/errs"
	"cfiber/pkg/logger"
)

// Tier 表示限流配额档位（对应 api.md §8）。
type Tier string

const (
	// TierSensitive 适用于登录、注册、重置密码等高风险认证端点。
	TierSensitive Tier = "sensitive"
	// TierAnonymous 适用于公开免鉴权接口（按客户端 IP 隔离）。
	TierAnonymous Tier = "anonymous"
	// TierAuthenticated 适用于已认证业务接口（优先按 UserID 隔离，未认证回退 IP）。
	TierAuthenticated Tier = "authenticated"
)

type throttleConfig struct {
	tier       Tier
	customKey  func(c fiber.Ctx) string
	onRejected fiber.Handler
}

// ThrottleOption 定制路由限流行为。
type ThrottleOption func(*throttleConfig)

// WithThrottleTier 指定限流所属业务档位。
func WithThrottleTier(tier Tier) ThrottleOption {
	return func(cfg *throttleConfig) { cfg.tier = tier }
}

// WithThrottleKey 自定义限流分区键提取函数。
func WithThrottleKey(fn func(c fiber.Ctx) string) ThrottleOption {
	return func(cfg *throttleConfig) { cfg.customKey = fn }
}

// WithThrottleResponse 自定义触发 429 超限时的响应处理器。
func WithThrottleResponse(handler fiber.Handler) ThrottleOption {
	return func(cfg *throttleConfig) { cfg.onRejected = handler }
}

// Throttle 按预设业务档位挂载 1 分钟滑动窗口限流中间件。
func (m *Middleware) Throttle(tier Tier) fiber.Handler {
	if !m.limiter.IsEnabled() {
		return func(c fiber.Ctx) error { return c.Next() }
	}
	return ThrottleWithStorage(m.storage, m.tierLimit(tier), time.Minute, WithThrottleTier(tier))
}

// ThrottleRate 按自定义配额与时间窗口挂载滑动窗口限流中间件。
func (m *Middleware) ThrottleRate(limit int, window time.Duration, opts ...ThrottleOption) fiber.Handler {
	if !m.limiter.IsEnabled() {
		return func(c fiber.Ctx) error { return c.Next() }
	}
	return ThrottleWithStorage(m.storage, limit, window, opts...)
}

// ThrottleWithStorage 基于官方 fiber/v3/middleware/limiter (SlidingWindow + fiber.Storage) 构建限流中间件。
func ThrottleWithStorage(storage fiber.Storage, limit int, window time.Duration, opts ...ThrottleOption) fiber.Handler {
	if storage == nil || limit <= 0 || window <= 0 {
		return func(c fiber.Ctx) error { return c.Next() }
	}

	cfg := &throttleConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	tierPrefix := string(cfg.tier)
	if tierPrefix == "" {
		tierPrefix = "custom"
	}
	limitStr := strconv.Itoa(limit)
	retryAfterSec := strconv.Itoa(max(int(window.Seconds()), 1))

	return limiter.New(limiter.Config{
		Max:                   limit,
		Expiration:            window,
		LimiterMiddleware:     limiter.SlidingWindow{},
		Storage:               failOpenStorage{storage: storage},
		DisableValueRedaction: true,
		KeyGenerator: func(c fiber.Ctx) string {
			return "ratelimit:" + tierPrefix + ":" + resolveScope(c, cfg)
		},
		LimitReached: func(c fiber.Ctx) error {
			scope := resolveScope(c, cfg)
			c.Set("X-RateLimit-Limit", limitStr)
			c.Set("X-RateLimit-Remaining", "0")
			if reset := c.GetRespHeader(fiber.HeaderRetryAfter); reset != "" {
				c.Set("X-RateLimit-Reset", reset)
			} else {
				c.Set(fiber.HeaderRetryAfter, retryAfterSec)
				c.Set("X-RateLimit-Reset", retryAfterSec)
			}

			ctx := c.Context()
			logger.FromContext(ctx).WarnContext(ctx, "rate limit exceeded",
				slog.String("tier", tierPrefix),
				slog.String("scope", scope),
				slog.Int("limit", limit),
				slog.String("path", c.Path()),
				slog.String("method", c.Method()),
				slog.String("ip", c.IP()),
			)

			if cfg.onRejected != nil {
				return cfg.onRejected(c)
			}
			return errs.RateLimited("Too many requests")
		},
	})
}

func (m *Middleware) tierLimit(tier Tier) int {
	switch tier {
	case TierSensitive:
		return m.limiter.Sensitive
	case TierAnonymous:
		return m.limiter.Anonymous
	case TierAuthenticated:
		return m.limiter.Authenticated
	default:
		return m.limiter.Anonymous
	}
}

func resolveScope(c fiber.Ctx, cfg *throttleConfig) string {
	if cfg.customKey != nil {
		if key := cfg.customKey(c); key != "" {
			return key
		}
	}
	if cfg.tier == TierAuthenticated {
		if id, ok := authentication.FromContext(c.Context()); ok && id.UserID != "" {
			return "user:" + id.UserID
		}
	}
	if ip := c.IP(); ip != "" {
		return "ip:" + ip
	}
	return "ip:unknown"
}

// failOpenStorage 对底层存储异常执行 fail-open 降级，避免存储抖动阻断正常业务流量。
type failOpenStorage struct {
	storage fiber.Storage
}

func (s failOpenStorage) Get(key string) ([]byte, error) {
	val, err := s.storage.Get(key)
	if err != nil {
		return nil, nil //nolint:nilerr // fail-open on storage error
	}
	return val, nil
}

func (s failOpenStorage) GetWithContext(ctx context.Context, key string) ([]byte, error) {
	val, err := s.storage.GetWithContext(ctx, key)
	if err != nil {
		return nil, nil //nolint:nilerr // fail-open on storage error
	}
	return val, nil
}

func (s failOpenStorage) Set(key string, val []byte, exp time.Duration) error {
	_ = s.storage.Set(key, val, exp)
	return nil
}

func (s failOpenStorage) SetWithContext(ctx context.Context, key string, val []byte, exp time.Duration) error {
	_ = s.storage.SetWithContext(ctx, key, val, exp)
	return nil
}

func (s failOpenStorage) Delete(key string) error {
	return s.storage.Delete(key)
}

func (s failOpenStorage) DeleteWithContext(ctx context.Context, key string) error {
	return s.storage.DeleteWithContext(ctx, key)
}

func (s failOpenStorage) Reset() error {
	return s.storage.Reset()
}

func (s failOpenStorage) ResetWithContext(ctx context.Context) error {
	return s.storage.ResetWithContext(ctx)
}

func (s failOpenStorage) Close() error {
	return s.storage.Close()
}

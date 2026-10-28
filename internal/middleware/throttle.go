package middleware

import (
	"cmp"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"

	"cfiber/pkg/authentication"
	"cfiber/pkg/errs"
	"cfiber/pkg/logger"
)

// Tier 定义业务限流预设档位。
type Tier string

const (
	// TierAnonymous 面向未认证客户端，按客户端 IP 计数。
	TierAnonymous Tier = "anonymous"
	// TierAuthenticated 面向已认证用户，按用户 ID 计数。
	TierAuthenticated Tier = "authenticated"
	// TierSensitive 面向敏感认证端点，按客户端 IP 计数且配额最严。
	TierSensitive Tier = "sensitive"

	rateLimitWindow = time.Minute
)

// ThrottleOption 定义自定义限流选项。
type ThrottleOption func(*throttleOptions)

type throttleOptions struct {
	keyFunc  func(c fiber.Ctx) string
	tier     Tier
	response func(c fiber.Ctx) error
}

// WithThrottleKey 自定义限流区分维度（例如按指定 header 或复合 key）。
func WithThrottleKey(fn func(c fiber.Ctx) string) ThrottleOption {
	return func(o *throttleOptions) {
		o.keyFunc = fn
	}
}

// WithThrottleTier 指定所属业务档位（用于监控与日志分类）。
func WithThrottleTier(tier Tier) ThrottleOption {
	return func(o *throttleOptions) {
		o.tier = tier
	}
}

// WithThrottleResponse 指定自定义限流触发时的响应拦截回调。
func WithThrottleResponse(fn func(c fiber.Ctx) error) ThrottleOption {
	return func(o *throttleOptions) {
		o.response = fn
	}
}

// Throttle 按档位预设配额执行平滑滑动窗口限流。
func (m *Middleware) Throttle(tier Tier, opts ...ThrottleOption) fiber.Handler {
	if m != nil && !m.limiter.IsEnabled() {
		return passthrough()
	}

	opts = append([]ThrottleOption{WithThrottleTier(tier)}, opts...)
	return ThrottleWithStorage(m.resolveLimiterStorage(), m.limitFor(tier), rateLimitWindow, opts...)
}

// ThrottleRate 按显式配额与时间周期执行平滑滑动窗口限流。
func (m *Middleware) ThrottleRate(limit int, window time.Duration, opts ...ThrottleOption) fiber.Handler {
	if m != nil && !m.limiter.IsEnabled() {
		return passthrough()
	}
	return ThrottleWithStorage(m.resolveLimiterStorage(), limit, window, opts...)
}

// passthrough 返回不做任何限流的空中间件。
func passthrough() fiber.Handler {
	return func(c fiber.Ctx) error { return c.Next() }
}

// resolveLimiterStorage 优先使用显式注入的存储，其次是默认 fiber.Storage。
func (m *Middleware) resolveLimiterStorage() fiber.Storage {
	if m == nil {
		return nil
	}
	return m.storage
}

// ThrottleWithStore 为测试与向后兼容保留的别名。
func ThrottleWithStore(storage fiber.Storage, limit int, window time.Duration, opts ...ThrottleOption) fiber.Handler {
	return ThrottleWithStorage(storage, limit, window, opts...)
}

// ThrottleWithStorage 基于 Fiber v3 官方 limiter.SlidingWindow 构造平滑滑动窗口限流中间件。
func ThrottleWithStorage(storage fiber.Storage, limit int, window time.Duration, opts ...ThrottleOption) fiber.Handler {
	if limit <= 0 {
		return passthrough()
	}

	var options throttleOptions
	for _, opt := range opts {
		opt(&options)
	}
	if window <= 0 {
		window = rateLimitWindow
	}

	tierName := throttleTierLabel(options.tier)

	var effectiveStorage fiber.Storage
	if storage != nil {
		effectiveStorage = failOpenStorage{storage: storage}
	}

	cfg := limiter.Config{
		Max:               limit,
		Expiration:        window,
		Storage:           effectiveStorage,
		LimiterMiddleware: limiter.SlidingWindow{},
		KeyGenerator: func(c fiber.Ctx) string {
			scope := resolveThrottleScope(c, options)
			return "ratelimit:" + tierName + ":" + scope
		},
		LimitReached: func(c fiber.Ctx) error {
			scope := resolveThrottleScope(c, options)
			c.Set("X-RateLimit-Limit", strconv.Itoa(limit))
			c.Set("X-RateLimit-Remaining", "0")
			if reset := c.GetRespHeader(fiber.HeaderRetryAfter); reset != "" {
				c.Set("X-RateLimit-Reset", reset)
			}

			logger.FromContext(c.Context()).Warn().
				Str("tier", tierName).
				Str("scope", scope).
				Int("limit", limit).
				Str("path", c.Path()).
				Str("method", c.Method()).
				Str("ip", c.IP()).
				Msg("rate limit exceeded")

			if options.response != nil {
				return options.response(c)
			}
			return errs.RateLimited("Too many requests")
		},
	}

	return limiter.New(cfg)
}

// resolveThrottleScope 解析限流区分维度：自定义键优先，其次业务档位，最后默认维度。
func resolveThrottleScope(c fiber.Ctx, options throttleOptions) string {
	switch {
	case options.keyFunc != nil:
		return options.keyFunc(c)
	case options.tier != "":
		return scopeKey(options.tier, c)
	default:
		return defaultScopeKey(c)
	}
}

// throttleTierLabel 返回日志用的档位名称，自定义限流记为 custom。
func throttleTierLabel(tier Tier) string {
	if tier == "" {
		return "custom"
	}
	return string(tier)
}

// scopeKey 根据档位语义决定隔离维度。
func scopeKey(tier Tier, c fiber.Ctx) string {
	switch tier {
	case TierAuthenticated:
		if identity, ok := authentication.FromContext(c.Context()); ok && identity.UserID != "" {
			return "user:" + identity.UserID
		}
		return defaultScopeKey(c)
	case TierAnonymous, TierSensitive:
		return defaultScopeKey(c)
	default:
		return defaultScopeKey(c)
	}
}

// failOpenStorage 对底层存储的异常进行兜底，确保缓存或网络抖动时不阻塞正常请求。
type failOpenStorage struct {
	storage fiber.Storage
}

func (s failOpenStorage) Get(key string) ([]byte, error) {
	val, err := s.storage.Get(key)
	if err != nil {
		return nil, nil //nolint:nilerr // fail-open on storage error to avoid dropping traffic
	}
	return val, nil
}

func (s failOpenStorage) GetWithContext(ctx context.Context, key string) ([]byte, error) {
	val, err := s.storage.GetWithContext(ctx, key)
	if err != nil {
		return nil, nil //nolint:nilerr // fail-open on storage error to avoid dropping traffic
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

// defaultScopeKey 默认按客户端真实 IP 隔离。
func defaultScopeKey(c fiber.Ctx) string {
	ip := strings.TrimSpace(c.IP())
	return "ip:" + cmp.Or(ip, "unknown")
}

// limitFor 按档位读取配额，未知档位回退至未认证配额。
func (m *Middleware) limitFor(tier Tier) int {
	switch tier {
	case TierAuthenticated:
		return m.limiter.Authenticated
	case TierSensitive:
		return m.limiter.Sensitive
	default:
		return m.limiter.Anonymous
	}
}

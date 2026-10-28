package global

import (
	"cmp"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/extractors"
	"github.com/gofiber/fiber/v3/middleware/session"
)

// DefaultSessionName 是默认 Session Cookie 名称。
const DefaultSessionName = "cfiber_session"

// defaultSessionMaxAge 是未配置有效期时的兜底（7 天）。
const defaultSessionMaxAge = 7 * 24 * time.Hour

// Session 基于 fiber/v3/middleware/session 提供会话管理。
func (m *Middleware) Session(name ...string) fiber.Handler {
	return session.New(session.Config{
		Store: m.SessionStore(name...),
	})
}

// SessionStore 返回当前 Session Store 实例。
func (m *Middleware) SessionStore(name ...string) *session.Store {
	sessionName := m.sessionName
	if len(name) > 0 && name[0] != "" {
		sessionName = name[0]
	}
	sessionName = cmp.Or(sessionName, DefaultSessionName)

	maxAge := m.sessionMaxAge
	if maxAge <= 0 {
		maxAge = defaultSessionMaxAge
	}

	if len(m.sessionSecret) == 0 {
		m.logger.Warn().
			Int("max_age_seconds", int(maxAge/time.Second)).
			Msg("session secret is empty, generated an ephemeral key: sessions will not survive a restart (set SESSION_SECRET)")
	}

	cfg := session.Config{
		Storage:        m.storage,
		Extractor:      extractors.FromCookie(sessionName),
		IdleTimeout:    maxAge,
		CookieSecure:   m.production,
		CookieHTTPOnly: true,
		CookieSameSite: "Lax",
		CookiePath:     "/",
	}

	return session.NewStore(cfg)
}

// SessionFromContext 从 Fiber 上下文获取当前活跃会话。
func SessionFromContext(c fiber.Ctx) *session.Middleware {
	return session.FromContext(c)
}

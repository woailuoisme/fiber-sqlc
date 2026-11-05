package v1

import (
	"time"

	clientrealtime "cfiber/internal/feature/client/realtime"
	platformRealtime "cfiber/pkg/realtime"
)

// TokenResource 描述客户端连接 Centrifugo 所需的 JWT 凭据与连接元信息。
type TokenResource struct {
	// Centrifugo 客户端连接 Token (JWT)
	Token string `json:"token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
	// Token 过期时间戳 (RFC3339)
	ExpiresAt time.Time `json:"expiresAt" example:"2026-09-27T19:30:00Z"`
	// Centrifugo WebSocket/SSE 服务端地址
	WSURL string `json:"wsUrl" example:"ws://127.0.0.1:8000/connection/websocket"`
}

// SubscribeBody 为 Centrifugo HTTP Proxy 订阅鉴权请求别名（供 Swagger 注解直接引用）。
type SubscribeBody = platformRealtime.SubscribeRequest

// SubscribeResource 为 Centrifugo HTTP Proxy 订阅鉴权响应别名（供 Swagger 注解直接引用）。
type SubscribeResource = platformRealtime.SubscribeResponse

// ToTokenResource 将服务层结果转换为契约响应。
func ToTokenResource(res *clientrealtime.TokenResult) *TokenResource {
	if res == nil {
		return nil
	}
	return &TokenResource{
		Token:     res.Token,
		ExpiresAt: res.ExpiresAt,
		WSURL:     res.WSURL,
	}
}

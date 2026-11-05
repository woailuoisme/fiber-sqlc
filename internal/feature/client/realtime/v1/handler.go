package v1

import (
	"cmp"

	"github.com/gofiber/fiber/v3"

	clientrealtime "cfiber/internal/feature/client/realtime"
	"cfiber/pkg/authentication"
	"cfiber/pkg/errs"
	platformRealtime "cfiber/pkg/realtime"
	"cfiber/pkg/response"
)

// Handler 是实时通信端点的 HTTP 适配层。
type Handler struct {
	svc *clientrealtime.RealtimeService
}

// NewHandler 构造 Handler。
func NewHandler(svc *clientrealtime.RealtimeService) *Handler {
	return &Handler{
		svc: svc,
	}
}

// GetToken 处理 GET /v1/realtime/token。
//
// @Summary      获取实时通信连接凭据
// @Description  为当前已认证用户签发直连 Centrifugo (WSS/SSE) 所需的 JWT 令牌，有效期由 CENTRIFUGO_TOKEN_TTL 决定（默认 2 小时）。
// @Tags         实时通信
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} TokenResource "实时通信连接凭据"
// @x-order      1
// @Router       /realtime/token [get]
func (h *Handler) GetToken(c fiber.Ctx) error {
	identity, err := authentication.RequireIdentity(c.Context())
	if err != nil {
		return err
	}

	res, err := h.svc.IssueToken(c.Context(), identity.UserID, identity.Role)
	if err != nil {
		return err
	}
	return response.OK(c, ToTokenResource(res))
}

// ProxySubscribe 处理 POST /v1/realtime/proxy/subscribe。
//
// @Summary      Centrifugo 订阅鉴权代理端点
// @Description  接收 Centrifugo 反向代理发起的频道订阅鉴权请求，校验 HMAC-SHA256 签名并调度频道权限策略。
// @Tags         实时通信
// @Accept       json
// @Produce      json
// @Param        payload body SubscribeBody true "Centrifugo 订阅代理请求"
// @Success      200 {object} SubscribeResource "Centrifugo 订阅代理响应"
// @x-order      2
// @Router       /realtime/proxy/subscribe [post]
func (h *Handler) ProxySubscribe(c fiber.Ctx) error {
	body := c.Body()

	sig := cmp.Or(
		c.Get(platformRealtime.CentrifugoSignatureHeader),
		c.Get(platformRealtime.CentrifugoSignatureLegacyHeader),
	)

	resp, err := h.svc.AuthorizeSubscribe(c.Context(), body, sig)
	if err != nil {
		status, mapped := errs.Map(err)
		return c.Status(status).JSON(fiber.Map{
			"error": fiber.Map{"code": status, "message": mapped.Message},
		})
	}

	return c.Status(fiber.StatusOK).JSON(resp)
}

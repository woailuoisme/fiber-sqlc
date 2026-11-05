package clientpayment

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/samber/oops"

	"cfiber/internal/config"
	"cfiber/pkg/cache"
	"cfiber/pkg/errs"
	"cfiber/pkg/eventbus"
	"cfiber/pkg/logger"
	"cfiber/pkg/payment"
)

const (
	// notifyLockTTL 是防止渠道并发通知重放的分布式锁持有时间。
	notifyLockTTL = 30 * time.Second
	// notifyDoneTTL 是已消费通知的持久去重标记保留时长。
	notifyDoneTTL = 7 * 24 * time.Hour
	// MsgUnsupportedChannel 是不支持支付渠道时的统一错误信息。
	MsgUnsupportedChannel = "Unsupported payment channel"

	purposeConsumed notificationKeyPurpose = "done"
	purposeLock     notificationKeyPurpose = "lock"
)

// notificationKeyPurpose 是幂等缓存键的用途维度：已消费标记与处理中锁必须分键，否则会互相覆盖。
type notificationKeyPurpose string

// CallbackHandler 是支付与退款业务消费钩子接口。
type CallbackHandler interface {
	OnPaymentSuccess(ctx context.Context, result *payment.PaymentResult) error
	OnRefundSuccess(ctx context.Context, result *payment.RefundResult) error
}

type defaultCallbackHandler struct{}

var _ CallbackHandler = (*defaultCallbackHandler)(nil)

func (d *defaultCallbackHandler) OnPaymentSuccess(ctx context.Context, result *payment.PaymentResult) error {
	logger.FromContext(ctx).ErrorContext(ctx, "payment success callback received without a registered handler",
		slog.String("out_trade_no", result.OutTradeNo),
		slog.String("channel_trade_no", result.ChannelTradeNo),
		slog.Int64("paid_amount", result.PaidAmount),
	)
	return errs.Internal(errors.New("payment callback handler is not configured"))
}

func (d *defaultCallbackHandler) OnRefundSuccess(ctx context.Context, result *payment.RefundResult) error {
	logger.FromContext(ctx).ErrorContext(ctx, "refund callback received without a registered handler",
		slog.String("out_trade_no", result.OutTradeNo),
		slog.String("out_refund_no", result.OutRefundNo),
		slog.String("channel_refund_no", result.ChannelRefundNo),
		slog.Int64("refund_amount", result.RefundAmount),
		slog.String("status", string(result.Status)),
	)
	return errs.Internal(errors.New("payment callback handler is not configured"))
}

// PaymentService 负责协调支付回调的分布式幂等防重与业务事件分发。
type PaymentService struct {
	gateway   payment.Gateway
	cache     cache.Store
	cfg       *config.Config
	publisher eventbus.Publisher
	handler   CallbackHandler
}

// NewPaymentService 构造支付回调协调服务。publisher 允许为 nil，表示不向外广播领域事件。
func NewPaymentService(
	gateway payment.Gateway,
	cacheClient cache.Store,
	cfg *config.Config,
	publisher eventbus.Publisher,
) *PaymentService {
	return &PaymentService{
		gateway:   gateway,
		cache:     cacheClient,
		cfg:       cfg,
		publisher: cmp.Or(publisher, eventbus.NopPublisher()),
		handler:   &defaultCallbackHandler{},
	}
}

// Gateway 暴露底层支付网关接口。
func (s *PaymentService) Gateway() payment.Gateway {
	if s == nil {
		return nil
	}
	return s.gateway
}

// SetCallbackHandler 注册业务消费钩子实现。
func (s *PaymentService) SetCallbackHandler(handler CallbackHandler) {
	if handler != nil {
		s.handler = handler
	}
}

// ProcessNotification 统一处理已通过传输层验签解析的支付/退款异步通知。
func (s *PaymentService) ProcessNotification(
	ctx context.Context,
	channel payment.Channel,
	notification *payment.Notification,
) (payment.NotificationAck, error) {
	log := logger.FromContext(ctx)

	if !channel.IsValid() {
		return payment.FailureAck(channel), errs.BadRequest(MsgUnsupportedChannel)
	}
	if notification == nil {
		return payment.FailureAck(channel), errs.BadRequest("notification payload is nil")
	}

	key := notificationKey{
		channel: channel,
		kind:    notification.Kind.String(),
		tradeNo: s.extractTradeNo(notification),
	}

	// 1. 幂等与并发防重放控制
	consumed, releaseLock, lockErr := s.acquireLockOrConsumed(ctx, key)
	if lockErr != nil {
		return payment.FailureAck(channel), lockErr
	}
	if consumed {
		return payment.SuccessAck(channel), nil
	}
	defer releaseLock()

	// 2. 分发到具体业务处理器与发布领域事件
	handleErr := s.dispatchNotification(ctx, channel, notification)
	if handleErr != nil {
		return payment.FailureAck(channel), oops.
			Code(errs.CodeInternal).
			With("channel", key.channel.String()).
			With("kind", key.kind).
			With("trade_no", key.tradeNo).
			Wrap(handleErr)
	}

	log.LogAttrs(ctx, slog.LevelInfo,
		"payment callback notification successfully processed",
		key.logAttrs()...,
	)

	// 3. 写入持久化去重标记
	s.recordConsumed(ctx, key)

	return payment.SuccessAck(channel), nil
}

type notificationKey struct {
	channel payment.Channel
	kind    string
	tradeNo string
}

func (k notificationKey) cacheKey(purpose notificationKeyPurpose) string {
	return cache.Key("payment", "notify", string(purpose), k.channel.String(), k.kind, k.tradeNo)
}

func (k notificationKey) logAttrs() []slog.Attr {
	return []slog.Attr{
		slog.String("channel", k.channel.String()),
		slog.String("kind", k.kind),
		slog.String("trade_no", k.tradeNo),
	}
}

func (s *PaymentService) acquireLockOrConsumed(ctx context.Context, key notificationKey) (bool, func(), error) {
	noop := func() {}
	if s.cache == nil || key.tradeNo == "" {
		return false, noop, nil
	}
	log := logger.FromContext(ctx)

	doneKey := key.cacheKey(purposeConsumed)
	if _, getErr := s.cache.Get(ctx, doneKey); getErr == nil {
		log.LogAttrs(ctx, slog.LevelWarn,
			"callback notification already consumed, acknowledged without reprocessing",
			key.logAttrs()...,
		)
		return true, noop, nil
	}

	lockKey := key.cacheKey(purposeLock)
	const lockToken = "1"
	acquired, lockErr := s.cache.SetNX(ctx, lockKey, lockToken, notifyLockTTL)
	if lockErr == nil && !acquired {
		return false, noop, errs.Conflict("duplicate notification in progress")
	}

	release := func() {
		unlockCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_, _ = s.cache.Delete(unlockCtx, lockKey)
	}
	return false, release, nil
}

func (s *PaymentService) dispatchNotification(ctx context.Context, channel payment.Channel, n *payment.Notification) error {
	switch n.Kind {
	case payment.NotificationKindPayment:
		if n.Payment == nil {
			return errs.BadRequest("payment notification payload is missing")
		}
		if err := s.handler.OnPaymentSuccess(ctx, n.Payment); err != nil {
			return err
		}
		_ = s.publisher.Publish(ctx, PaymentSucceededEvent{
			Channel:   channel,
			Payment:   n.Payment,
			Timestamp: time.Now(),
		})
		return nil

	case payment.NotificationKindRefund:
		if n.Refund == nil {
			return errs.BadRequest("refund notification payload is missing")
		}
		if err := s.handler.OnRefundSuccess(ctx, n.Refund); err != nil {
			return err
		}
		_ = s.publisher.Publish(ctx, PaymentRefundedEvent{
			Channel:   channel,
			Refund:    n.Refund,
			Timestamp: time.Now(),
		})
		return nil

	default:
		return errs.BadRequest("unsupported payment notification kind: " + n.Kind.String())
	}
}

func (s *PaymentService) recordConsumed(ctx context.Context, key notificationKey) {
	if s.cache == nil || key.tradeNo == "" {
		return
	}
	doneKey := key.cacheKey(purposeConsumed)
	if err := s.cache.Set(ctx, doneKey, "1", notifyDoneTTL); err != nil {
		logger.FromContext(ctx).WarnContext(ctx, "failed to persist payment notification dedup marker",
			slog.Any("error", err),
			slog.String("trade_no", key.tradeNo),
		)
	}
}

func (s *PaymentService) baseURL() string {
	if s == nil || s.cfg == nil {
		return ""
	}
	return strings.TrimRight(s.cfg.App.URL, "/")
}

func (s *PaymentService) FailureRedirectURL() string {
	return s.baseURL() + "/orders?status=failed"
}

func (s *PaymentService) BuildAlipayReturnURL(outTradeNo string) string {
	return fmt.Sprintf("%s/orders/%s?status=success", s.baseURL(), outTradeNo)
}

func (s *PaymentService) extractTradeNo(n *payment.Notification) string {
	if n == nil {
		return ""
	}
	if n.Payment != nil && n.Payment.OutTradeNo != "" {
		return n.Payment.OutTradeNo
	}
	if n.Refund != nil && n.Refund.OutRefundNo != "" {
		return n.Refund.OutRefundNo
	}

	return n.ID
}

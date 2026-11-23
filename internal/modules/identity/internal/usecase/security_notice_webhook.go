package usecase

import (
	"context"
	"time"
)

// VerifiedSecurityNoticeReceipt carries only fixed-length digests of a signed
// Mailgun event's replay token, event identity and provider message identity.
// The signed webhook body is never persisted: it contains recipient PII.
type VerifiedSecurityNoticeReceipt struct {
	TokenDigest   [32]byte
	EventDigest   [32]byte
	MessageDigest [32]byte
	DeliveryState string
	EventAt       time.Time
}

// SecurityNoticeWebhookRecorder is an isolated, append-only, EXECUTE-only SQL
// capability. The HTTP signature must be verified before calling Record.
type SecurityNoticeWebhookRecorder interface {
	Record(context.Context, VerifiedSecurityNoticeReceipt) error
}

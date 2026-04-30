package notifications

import (
	"time"
)

// Type matches Notification.type values produced by the Next.js admin/web app.
type Type string

const (
	TypeCommentReply   Type = "COMMENT_REPLY"
	TypePostComment    Type = "POST_COMMENT"
	TypeAdminBroadcast Type = "ADMIN_BROADCAST"
)

// Fanout matches Notification.fanout. WRITE = inbox row per user;
// READ = no inbox rows, push only (topic or multicast).
type Fanout string

const (
	FanoutWrite Fanout = "WRITE"
	FanoutRead  Fanout = "READ"
)

// Status matches Notification.status. The worker only writes:
//   pending → sending → sent | failed
// 'canceled' is set by the admin app and is ignored by the claim query.
type Status string

const (
	StatusPending  Status = "pending"
	StatusSending  Status = "sending"
	StatusSent     Status = "sent"
	StatusFailed   Status = "failed"
	StatusCanceled Status = "canceled"
)

// AudienceType describes the population a READ-fanout broadcast targets.
type AudienceType string

const (
	AudienceTenant   AudienceType = "tenant"   // entire tenant — sent via FCM topic
	AudienceDelivery AudienceType = "delivery" // members of one delivery — multicast
)

// Notification is a row from the "Notification" table, scanned with the
// columns the worker needs.
type Notification struct {
	ID       string
	TenantID string
	Type     Type
	Fanout   Fanout
	Status   Status

	// Push body. Either (Title, Body) — broadcasts — or (MessageKey,
	// MessageData) — system notifications. Worker resolves to a final
	// (title, body) pair via render.go.
	Title       *string
	Body        *string
	MessageKey  *string
	MessageData []byte

	// Audience (only meaningful for fanout=READ).
	AudienceType *string
	AudienceID   *string

	// Progress.
	RecipientCount *int
	SentCount      int
	FailedCount    int
	LastBatchIndex *int

	ScheduledAt *time.Time
	UpdatedAt   time.Time
}

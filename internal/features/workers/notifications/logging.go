package notifications

import "github.com/memberclass-backend-golang/internal/domain/ports"

// dispatchLog wraps ports.Logger and prepends a fixed set of fields on
// every line, so every log emitted during a single dispatch carries the
// same correlation keys (notification_id, tenant_id, type, fanout,
// audience_type). Filtering Datadog/Railway logs by `notification_id:<id>`
// returns the entire lifecycle of one push, including the failure cause.
type dispatchLog struct {
	log  ports.Logger
	base []any
}

func newDispatchLog(log ports.Logger, n Notification) *dispatchLog {
	return &dispatchLog{
		log: log,
		base: []any{
			"notification_id", n.ID,
			"tenant_id", n.TenantID,
			"type", string(n.Type),
			"fanout", string(n.Fanout),
			"audience_type", deref(n.AudienceType),
		},
	}
}

// merge returns base + extras as a single slice. We always allocate a
// fresh slice (instead of `append(d.base, extras...)`) so concurrent calls
// can't observe each other's appended fields when base happens to share
// a backing array.
func (d *dispatchLog) merge(extras []any) []any {
	out := make([]any, 0, len(d.base)+len(extras))
	out = append(out, d.base...)
	out = append(out, extras...)
	return out
}

func (d *dispatchLog) Info(msg string, kv ...any)  { d.log.Info(msg, d.merge(kv)...) }
func (d *dispatchLog) Warn(msg string, kv ...any)  { d.log.Warn(msg, d.merge(kv)...) }
func (d *dispatchLog) Error(msg string, kv ...any) { d.log.Error(msg, d.merge(kv)...) }

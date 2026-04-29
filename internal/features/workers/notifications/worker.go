package notifications

import (
	"context"
	"errors"
	"fmt"
	"time"

	"firebase.google.com/go/v4/messaging"
)

// Tunables. These are package-level constants so tests can read them; if
// they ever need to be per-tenant they should move into Feature.
const (
	// FCM multicast hard limit is 500 tokens per call.
	batchSize = 500

	// Long-poll cadence — the cost of polling is one cheap UPDATE…RETURNING
	// per tick, and 10s lag on a push is fine.
	pollInterval = 10 * time.Second

	// How often to scan for crashed-mid-send rows.
	orphanInterval = 1 * time.Minute

	// Per-tick claim cap. Keeps a slow tenant from monopolizing the worker —
	// remaining rows wait for the next tick (10s).
	claimLimit = 50
)

// Start kicks off the worker goroutine. Idempotent — calling twice is a
// no-op the second time. Wire it from cmd/api/main.go's startApplication
// AFTER the DB is open and BEFORE the HTTP server starts accepting work,
// so push delivery is live as soon as the server is.
func (f *Feature) Start(parent context.Context) {
	f.mu.Lock()
	if f.running {
		f.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	f.cancel = cancel
	f.done = make(chan struct{})
	f.running = true
	f.mu.Unlock()

	go func() {
		defer close(f.done)
		f.run(ctx)
	}()
}

// Stop signals the worker to drain and waits up to `timeout` for the
// current tick to finish. Call from graceful shutdown BEFORE closing the DB.
func (f *Feature) Stop(timeout time.Duration) {
	f.mu.Lock()
	if !f.running {
		f.mu.Unlock()
		return
	}
	cancel, done := f.cancel, f.done
	f.running = false
	f.mu.Unlock()

	cancel()
	select {
	case <-done:
	case <-time.After(timeout):
		f.log.Warn("notifications.worker: shutdown timed out; in-flight rows recovered by orphan reset on next boot")
	}
}

func (f *Feature) run(ctx context.Context) {
	pollT := time.NewTicker(pollInterval)
	orphanT := time.NewTicker(orphanInterval)
	defer pollT.Stop()
	defer orphanT.Stop()

	f.log.Info("notifications.worker: started")

	for {
		select {
		case <-ctx.Done():
			f.log.Info("notifications.worker: stopped")
			return
		case <-orphanT.C:
			n, err := f.resetOrphans(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				f.log.Error("notifications.worker.orphan_reset_failed", "error", err.Error())
			} else if n > 0 {
				f.log.Info("notifications.worker.orphans_reset", "count", n)
			}
		case <-pollT.C:
			if err := f.tick(ctx); err != nil && !errors.Is(err, context.Canceled) {
				f.log.Error("notifications.worker.tick_failed", "error", err.Error())
			}
		}
	}
}

func (f *Feature) tick(ctx context.Context) error {
	notifs, err := f.claimPending(ctx, claimLimit)
	if err != nil {
		return err
	}
	for _, n := range notifs {
		if err := f.dispatch(ctx, n); err != nil {
			f.log.Error("notifications.worker.dispatch_failed",
				"id", n.ID, "tenant_id", n.TenantID, "error", err.Error())
			if mErr := f.markFailed(ctx, n.ID, err.Error()); mErr != nil {
				f.log.Error("notifications.worker.mark_failed_failed",
					"id", n.ID, "error", mErr.Error())
			}
		}
	}
	return nil
}

// dispatch routes one Notification to FCM topic or multicast based on
// fanout/audience. It never returns nil for "I tried but the row is now
// failed" — the caller checks error and writes the failed row itself.
func (f *Feature) dispatch(ctx context.Context, n Notification) error {
	instance, err := f.getTenantInstance(ctx, n.TenantID)
	if err != nil {
		return fmt.Errorf("get tenant instance: %w", err)
	}

	sender, _, err := f.fcm.messaging(ctx, instance)
	if err != nil {
		return fmt.Errorf("fcm client: %w", err)
	}

	title, body := renderForPush(n)

	if n.Fanout == FanoutRead && deref(n.AudienceType) == string(AudienceTenant) {
		return f.sendTopic(ctx, sender, n, title, body)
	}
	return f.sendMulticast(ctx, sender, n, title, body)
}

// sendTopic publishes one FCM message to the tenant_<tenantId> topic. The
// app subscribes/unsubscribes devices to this topic on its end. We don't
// get per-recipient stats — sentCount/failedCount stay at 0 and
// recipientCount is set to the UsersOnTenants count as an estimate.
func (f *Feature) sendTopic(ctx context.Context, sender fcmSender, n Notification, title, body string) error {
	topic := "tenant_" + n.TenantID
	if _, err := sender.Send(ctx, &messaging.Message{
		Topic:        topic,
		Notification: &messaging.Notification{Title: title, Body: body},
	}); err != nil {
		return fmt.Errorf("fcm topic send: %w", err)
	}

	if n.RecipientCount == nil {
		if rc, err := f.countTenantMembers(ctx, n.TenantID); err == nil {
			if err := f.setRecipientCount(ctx, n.ID, rc); err != nil {
				f.log.Warn("notifications.worker.set_recipient_count_failed",
					"id", n.ID, "error", err.Error())
			}
		}
	}
	return f.markSent(ctx, n.ID, 0, 0)
}

// sendMulticast resolves the recipient list, chunks at 500 tokens, and
// updates lastBatchIndex after each chunk so a crash mid-broadcast resumes
// without duplicating sends.
func (f *Feature) sendMulticast(ctx context.Context, sender fcmSender, n Notification, title, body string) error {
	recipients, err := f.resolveRecipients(ctx, n)
	if err != nil {
		return fmt.Errorf("resolve recipients: %w", err)
	}
	if len(recipients) == 0 {
		// Nothing to send — could be a broadcast for a delivery with no
		// members, or a personal notification for a user with no devices.
		// Mark as sent with 0/0 so admins see the row is closed.
		if n.RecipientCount == nil {
			_ = f.setRecipientCount(ctx, n.ID, 0)
		}
		return f.markSent(ctx, n.ID, 0, 0)
	}

	if n.RecipientCount == nil {
		if err := f.setRecipientCount(ctx, n.ID, len(recipients)); err != nil {
			f.log.Warn("notifications.worker.set_recipient_count_failed",
				"id", n.ID, "error", err.Error())
		}
	}

	// Resume from after lastBatchIndex if we crashed earlier. The index is
	// 0-based and inclusive (the *last* batch we actually finished), so we
	// start at index+1.
	startBatch := 0
	if n.LastBatchIndex != nil {
		startBatch = *n.LastBatchIndex + 1
	}

	sent := n.SentCount
	failed := n.FailedCount

	for batchIdx, i := startBatch, startBatch*batchSize; i < len(recipients); batchIdx, i = batchIdx+1, i+batchSize {
		end := min(i+batchSize, len(recipients))
		chunk := recipients[i:end]

		tokens := make([]string, len(chunk))
		for k, r := range chunk {
			tokens[k] = r.token
		}

		resp, err := sender.SendEachForMulticast(ctx, &messaging.MulticastMessage{
			Tokens:       tokens,
			Notification: &messaging.Notification{Title: title, Body: body},
		})
		if err != nil {
			return fmt.Errorf("fcm multicast batch %d: %w", batchIdx, err)
		}

		sent += resp.SuccessCount
		failed += resp.FailureCount

		// Best-effort: drop tokens FCM said are dead. If the user reinstalls
		// the app it'll register a new token via NotificationDevice anyway.
		for k, r := range resp.Responses {
			if r.Error != nil && messaging.IsUnregistered(r.Error) {
				if dErr := f.deleteDevice(ctx, chunk[k].userID, n.TenantID, tokens[k]); dErr != nil {
					f.log.Warn("notifications.worker.delete_device_failed",
						"user_id", chunk[k].userID, "error", dErr.Error())
				}
			}
		}

		if err := f.updateProgress(ctx, n.ID, sent, failed, batchIdx); err != nil {
			f.log.Warn("notifications.worker.progress_update_failed",
				"id", n.ID, "error", err.Error())
		}
	}

	if sent == 0 && failed > 0 {
		return fmt.Errorf("all %d FCM sends failed", failed)
	}
	return f.markSent(ctx, n.ID, sent, failed)
}

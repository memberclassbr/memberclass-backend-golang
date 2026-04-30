package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

// fcmSender is the subset of *messaging.Client the worker needs. Defining
// it as an interface lets tests substitute a fake without touching the
// Firebase SDK.
type fcmSender interface {
	Send(ctx context.Context, message *messaging.Message) (string, error)
	SendEachForMulticast(ctx context.Context, message *messaging.MulticastMessage) (*messaging.BatchResponse, error)
}

// fcmClient is a slice-local cache of *firebase.App keyed by Firebase project
// id. The Admin SDK warns about creating multiple apps for the same project,
// so we memoize.
type fcmClient struct {
	mu   sync.Mutex
	apps map[string]*firebase.App

	// senderFactory builds an fcmSender from a *firebase.App. Overridden
	// in tests; defaults to (*firebase.App).Messaging.
	senderFactory func(ctx context.Context, app *firebase.App) (fcmSender, error)
}

func newFCMClient() *fcmClient {
	return &fcmClient{
		apps: map[string]*firebase.App{},
		senderFactory: func(ctx context.Context, app *firebase.App) (fcmSender, error) {
			return app.Messaging(ctx)
		},
	}
}

// messaging returns an fcmSender for the Firebase project configured for
// `notificationsInstance` (Tenant.notificationsInstance). The mapping
// mirrors the Next.js `getFirebaseConfig` selector so both sides hit the
// same project for the same tenant.
func (c *fcmClient) messaging(ctx context.Context, notificationsInstance string) (fcmSender, string, error) {
	saJSON, projectID, err := selectFirebaseKey(notificationsInstance)
	if err != nil {
		return nil, "", err
	}

	c.mu.Lock()
	app, ok := c.apps[projectID]
	if !ok {
		cfg := &firebase.Config{ProjectID: projectID}
		a, err := firebase.NewApp(ctx, cfg, option.WithCredentialsJSON(saJSON))
		if err != nil {
			c.mu.Unlock()
			return nil, "", fmt.Errorf("firebase.NewApp(%s): %w", projectID, err)
		}
		c.apps[projectID] = a
		app = a
	}
	c.mu.Unlock()

	sender, err := c.senderFactory(ctx, app)
	if err != nil {
		return nil, "", fmt.Errorf("firebase.Messaging(%s): %w", projectID, err)
	}
	return sender, projectID, nil
}

// selectFirebaseKey chooses which FIREBASE_SERVICE_ACCOUNT_KEY_* env var to
// use. The Next.js side uses the same mapping; KEEP IN SYNC.
//
//	memberclass-0825 → FIREBASE_SERVICE_ACCOUNT_KEY_2
//	mcpush3-87886    → FIREBASE_SERVICE_ACCOUNT_KEY_3
//	mcpush4-d0d86    → FIREBASE_SERVICE_ACCOUNT_KEY_4
//	other / null     → FIREBASE_SERVICE_ACCOUNT_KEY (default project)
func selectFirebaseKey(notificationsInstance string) ([]byte, string, error) {
	var (
		envVar    string
		projectID string
	)
	switch notificationsInstance {
	case "memberclass-0825":
		envVar = "FIREBASE_SERVICE_ACCOUNT_KEY_2"
		projectID = "memberclass-0825"
	case "mcpush3-87886":
		envVar = "FIREBASE_SERVICE_ACCOUNT_KEY_3"
		projectID = "mcpush3-87886"
	case "mcpush4-d0d86":
		envVar = "FIREBASE_SERVICE_ACCOUNT_KEY_4"
		projectID = "mcpush4-d0d86"
	default:
		envVar = "FIREBASE_SERVICE_ACCOUNT_KEY"
		if notificationsInstance != "" {
			projectID = notificationsInstance
		} else {
			projectID = "memberclass-84a92"
		}
	}

	raw := os.Getenv(envVar)
	if raw == "" {
		return nil, "", fmt.Errorf("env %s not set", envVar)
	}
	var probe map[string]any
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return nil, "", fmt.Errorf("env %s is not valid JSON: %w", envVar, err)
	}
	return []byte(raw), projectID, nil
}

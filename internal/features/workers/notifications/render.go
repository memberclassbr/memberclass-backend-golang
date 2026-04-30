package notifications

import (
	"encoding/json"
	"fmt"
	"strings"
)

// pushTemplate is the pt-BR text for a system notification key. Broadcasts
// don't use this — they ship Title/Body literal strings.
type pushTemplate struct {
	title string
	body  string
}

// ptBR mirrors the Next.js `messages/pt-BR.json` push entries. KEEP IN SYNC
// when adding new system notification types — both sides need the key.
//
// The {placeholder} tokens are interpolated from Notification.messageData,
// which is a JSON object the writer (Next.js) already validated.
var ptBR = map[string]pushTemplate{
	"notifications.commentReply": {
		title: "Sua dúvida foi respondida",
		body:  `Sua pergunta na aula "{lessonName}" foi respondida.`,
	},
	"notifications.postComment": {
		title: "Comentaram no seu post",
		body:  "{actorName} comentou no seu post da comunidade.",
	},
}

// renderForPush picks the title/body that will appear on the device's
// notification tray.
//
//   - Broadcasts (admin) carry literal Title/Body — used as-is.
//   - System notifications carry MessageKey + MessageData — looked up in
//     `ptBR` and interpolated.
//
// MVP is pt-BR only. Per-device locale is a future enhancement; the app
// can already re-render in the user's language when it opens because
// MessageData is forwarded as the FCM data payload elsewhere.
func renderForPush(n Notification) (title, body string) {
	if n.Title != nil && n.Body != nil {
		return *n.Title, *n.Body
	}
	if n.MessageKey == nil {
		return "", ""
	}
	tpl, ok := ptBR[*n.MessageKey]
	if !ok {
		// Fallback: at least show the key so a missing translation is
		// visible in QA instead of an empty notification.
		return *n.MessageKey, ""
	}
	title = tpl.title
	body = tpl.body
	if len(n.MessageData) == 0 {
		return
	}
	var data map[string]any
	if err := json.Unmarshal(n.MessageData, &data); err != nil {
		return
	}
	for k, v := range data {
		placeholder := "{" + k + "}"
		val := fmt.Sprint(v)
		title = strings.ReplaceAll(title, placeholder, val)
		body = strings.ReplaceAll(body, placeholder, val)
	}
	return
}

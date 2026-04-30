package notifications

import (
	"testing"
)

func ptr(s string) *string { return &s }

func TestRenderForPush(t *testing.T) {
	tests := []struct {
		name      string
		n         Notification
		wantTitle string
		wantBody  string
	}{
		{
			name: "broadcast uses literal title/body",
			n: Notification{
				Title: ptr("Aviso"),
				Body:  ptr("Estamos em manutenção"),
			},
			wantTitle: "Aviso",
			wantBody:  "Estamos em manutenção",
		},
		{
			name: "system commentReply with placeholder",
			n: Notification{
				MessageKey:  ptr("notifications.commentReply"),
				MessageData: []byte(`{"lessonName":"Aula 1"}`),
			},
			wantTitle: "Sua dúvida foi respondida",
			wantBody:  `Sua pergunta na aula "Aula 1" foi respondida.`,
		},
		{
			name: "missing key falls back to key as title",
			n: Notification{
				MessageKey: ptr("notifications.unknown"),
			},
			wantTitle: "notifications.unknown",
			wantBody:  "",
		},
		{
			name: "system without messageData leaves placeholders raw",
			n: Notification{
				MessageKey: ptr("notifications.postComment"),
			},
			wantTitle: "Comentaram no seu post",
			wantBody:  "{actorName} comentou no seu post da comunidade.",
		},
		{
			name:      "no title, no key returns empty",
			n:         Notification{},
			wantTitle: "",
			wantBody:  "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotT, gotB := renderForPush(tc.n)
			if gotT != tc.wantTitle {
				t.Errorf("title: got %q want %q", gotT, tc.wantTitle)
			}
			if gotB != tc.wantBody {
				t.Errorf("body: got %q want %q", gotB, tc.wantBody)
			}
		})
	}
}

package alerting

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/email"
)

type capSender struct{ plain, rich int }

func (c *capSender) Send(context.Context, string, string, string) error { c.plain++; return nil }
func (c *capSender) SendMessage(context.Context, email.Message) error   { c.rich++; return nil }

func TestSendTemplated(t *testing.T) {
	ctx := context.Background()
	plain := &capSender{}
	if err := sendTemplated(ctx, plain, "alert_notification", "a@b.c", nil, "s", "b"); err != nil || plain.plain != 1 {
		t.Fatalf("plain sender: %v %+v", err, plain)
	}
	rich := &capSender{}
	ts := TemplatedSender{Sender: rich, Render: func(context.Context, string, string, map[string]string) (email.Message, bool) {
		return email.Message{To: "a@b.c", Subject: "s", Text: "t", HTML: "<p>t</p>"}, true
	}}
	if err := sendTemplated(ctx, ts, "alert_notification", "a@b.c", nil, "s", "b"); err != nil || rich.rich != 1 {
		t.Fatalf("templated sender: %v %+v", err, rich)
	}
	miss := &capSender{}
	ts = TemplatedSender{Sender: miss, Render: func(context.Context, string, string, map[string]string) (email.Message, bool) {
		return email.Message{}, false
	}}
	if err := sendTemplated(ctx, ts, "x", "a@b.c", nil, "s", "b"); err != nil || miss.plain != 1 {
		t.Fatalf("fallback: %v %+v", err, miss)
	}
}

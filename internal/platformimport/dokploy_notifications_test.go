package platformimport

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

var notificationFixtureSecrets = []string{"FIXTURE-SLACK-TOKEN", "fixture-bot-token", "fixture-secret-topic", "fixture-ntfy-token", "fixture-smtp-password", "fixture-matrix-token"}

func TestDokployNotificationsDiscoveredWithoutSecrets(t *testing.T) {
	srv := readOnlyServer(t, map[string]string{
		"/api/project.all":      "dokploy/project_backups.json",
		"/api/notification.all": "dokploy/notifications.json",
	}, "", "x-api-key", "key-fixture")
	src, err := NewDokploy(srv.URL, "key-fixture", testOpts)
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Notifications) != 5 {
		t.Fatalf("got %d providers, want 5", len(d.Notifications))
	}
	dump := fmt.Sprintf("%+v", d.Notifications)
	for _, s := range notificationFixtureSecrets {
		if strings.Contains(dump, s) {
			t.Fatalf("discovery holds a secret: %s", dump)
		}
	}
	byID := map[string]NotificationProvider{}
	for _, p := range d.Notifications {
		byID[p.SourceID] = p
	}
	if !byID["ntf-slack"].HasSecret || !byID["ntf-tg"].HasSecret || !byID["ntf-ntfy"].HasAccessToken || byID["ntf-ntfy"].Server != "https://ntfy.example.com" {
		t.Fatalf("provider flags wrong: %+v", d.Notifications)
	}
	if byID["ntf-mail"].Recipient != "oncall@example.com" || byID["ntf-matrix"].Kind != "matrix" {
		t.Fatalf("email or unsupported provider wrong: %+v", d.Notifications)
	}
	got, err := src.NotificationTarget(context.Background(), "ntf-tg")
	if err != nil || got.URL != "https://api.telegram.org/botfixture-bot-token/sendMessage?chat_id=-1001" || got.Kind != "telegram" {
		t.Fatalf("telegram target = %q, %v", got, err)
	}
	if got, err := src.NotificationTarget(context.Background(), "ntf-ntfy"); err != nil || got.URL != "https://ntfy.example.com/fixture-secret-topic?auth=fixture-ntfy-token" || got.Server != "https://ntfy.example.com" {
		t.Fatalf("ntfy target = %q, %v", got, err)
	}
	if _, err := src.NotificationTarget(context.Background(), "ntf-mail"); err == nil {
		t.Fatal("email has no secret target")
	}
}

func TestValidateNotifyEndpointNeverEchoesURL(t *testing.T) {
	for _, raw := range []string{"http://hooks.example.com/T0K3N-SECRET", "https://u:T0K3N-SECRET@hooks.example.com/x", "https://127.0.0.1/T0K3N-SECRET", "not a url T0K3N-SECRET"} {
		err := ValidateNotifyEndpoint(context.Background(), raw, NetworkPolicy{})
		if err == nil || strings.Contains(err.Error(), "T0K3N-SECRET") {
			t.Fatalf("%q: err = %v", raw, err)
		}
	}
}

func TestNotificationReadsOnlyKindObject(t *testing.T) {
	var mixed []dokployNotification
	if err := json.Unmarshal([]byte(`[
	 {"notificationId":"a","name":"m","notificationType":"email","custom":{"endpoint":"https://hooks.example.com/x"}},
	 {"notificationId":"b","name":"t","notificationType":"telegram","telegram":{"botToken":"tg-fixture","chatId":"7"},"slack":{"webhookUrl":"https://hooks.example.com/slack-fixture"}},
	 {"notificationId":"c","name":"c","notificationType":"custom","email":{"toAddresses":["a@example.com"]}}
	]`), &mixed); err != nil {
		t.Fatal(err)
	}
	if p := describeNotification(mixed[0]); !p.Mismatch || p.HasSecret || p.Recipient != "" {
		t.Fatalf("email with a custom object: %+v", p)
	}
	if p := describeNotification(mixed[2]); !p.Mismatch || p.HasSecret || p.Recipient != "" {
		t.Fatalf("custom with an email object: %+v", p)
	}
	p := describeNotification(mixed[1])
	tgt, err := buildNotificationTarget(mixed[1])
	if p.Mismatch || err != nil || !strings.Contains(tgt.URL, "api.telegram.org/bottg-fixture/") || strings.Contains(tgt.URL, "slack") {
		t.Fatalf("telegram with a slack object: %+v %q %v", p, tgt.URL, err)
	}
}

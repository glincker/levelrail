package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_EventWebhooksCreatePrintsSecretOnce(t *testing.T) {
	var calls []exportCall
	srv := newExportServer(t, &calls, func(*http.Request) (int, any) {
		return 201, apiclient.EventWebhookResource{ID: "whk_1", Name: "siem", SigningSecret: "whsec_abc"}
	})
	stdout, _ := runCLIExpectOK(t, []string{
		"event-webhooks", "create", "--name", "siem", "--url", "https://203.0.113.5/hook",
		"--event", "apps.*,users.create", "--api-url", srv.URL,
	})
	var req apiclient.CreateEventWebhookRequest
	if err := json.Unmarshal([]byte(calls[0].body), &req); err != nil {
		t.Fatal(err)
	}
	if calls[0].path != "/api/v1/event-webhooks" || len(req.Events) != 2 || req.Enabled == nil || !*req.Enabled {
		t.Fatalf("call = %+v req = %+v", calls[0], req)
	}
	if !strings.Contains(stdout, "whsec_abc") {
		t.Fatalf("stdout %q missing the signing secret", stdout)
	}
}

func TestRun_EventWebhooksReplayAndDeliveries(t *testing.T) {
	var calls []exportCall
	srv := newExportServer(t, &calls, func(r *http.Request) (int, any) {
		if r.Method == http.MethodPost {
			return 200, apiclient.EventDeliveryResource{ID: "dlv_1", State: "pending"}
		}
		return 200, []apiclient.EventDeliveryResource{{ID: "dlv_1", EventType: "apps.deploy", State: "dead", Attempts: 8, ResponseStatus: 502, LastError: "endpoint answered HTTP 502"}}
	})
	stdout, _ := runCLIExpectOK(t, []string{"event-webhooks", "deliveries", "whk_1", "--api-url", srv.URL})
	for _, want := range []string{"dlv_1", "apps.deploy", "dead", "502"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q missing %q", stdout, want)
		}
	}
	runCLIExpectOK(t, []string{"event-webhooks", "replay", "dlv_1", "--api-url", srv.URL})
	if last := calls[len(calls)-1]; last.method != http.MethodPost || last.path != "/api/v1/event-webhook-deliveries/dlv_1/replay" {
		t.Fatalf("replay call = %+v", last)
	}
}

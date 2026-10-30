package apiclient

import (
	"encoding/json"
	"testing"
)

func TestPublishedPortsDecode(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []PublishedPort
	}{
		{"app udp", `{"name":"dns","published_ports":[{"container_port":53,"host_port":53,"protocol":"udp"}]}`, []PublishedPort{{53, 53, "udp"}}},
		{"absent", `{"name":"web"}`, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var app AppResource
			if err := json.Unmarshal([]byte(tc.body), &app); err != nil {
				t.Fatal(err)
			}
			if len(app.PublishedPorts) != len(tc.want) {
				t.Fatalf("got %v want %v", app.PublishedPorts, tc.want)
			}
			for i := range tc.want {
				if app.PublishedPorts[i] != tc.want[i] {
					t.Fatalf("got %v want %v", app.PublishedPorts, tc.want)
				}
			}
		})
	}
	var res ComposeDeployResult
	body := `{"app_id":"a","services":[{"name":"dns","published_ports":[{"container_port":53,"host_port":53,"protocol":"udp"}]}]}`
	if err := json.Unmarshal([]byte(body), &res); err != nil || len(res.Services[0].PublishedPorts) != 1 {
		t.Fatalf("compose services lost ports: %v %v", err, res)
	}
}

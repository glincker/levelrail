package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintWALShip(t *testing.T) {
	tests := []struct {
		name string
		ship *apiclient.WALShipResource
		want string
	}{
		{"none yet", nil, "not shipped yet"},
		{"healthy", &apiclient.WALShipResource{LastSuccessAt: "2026-10-05T00:00:00Z", Shipped: 4}, "last shipped 2026-10-05T00:00:00Z (4 segments"},
		{"failing", &apiclient.WALShipResource{LastAttemptAt: "t1", LastError: "bucket down"}, "FAILING since t1: bucket down"},
	}
	for _, tt := range tests {
		var buf bytes.Buffer
		printWALShip(&buf, tt.ship)
		if !strings.Contains(buf.String(), tt.want) {
			t.Errorf("%s: output %q missing %q", tt.name, buf.String(), tt.want)
		}
	}
}

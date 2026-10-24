package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintCPHealthReadQueue(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    string
	}{
		{"disabled", false, "read pool off, reads use the writer"},
		{"enabled", true, "3 waits, 12 ms total, 2 of 4 connections busy"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var h apiclient.CPHealth
			h.DB.ReadPoolEnabled = tc.enabled
			h.DB.ReadPoolSize, h.DB.ReadInUse, h.DB.ReadWaitCount, h.DB.ReadWaitMS = 4, 2, 3, 12
			var out bytes.Buffer
			printCPHealth(&out, h)
			if !strings.Contains(out.String(), "read queue:  "+tc.want) {
				t.Fatalf("output lacks %q:\n%s", tc.want, out.String())
			}
		})
	}
}

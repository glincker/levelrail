package main

import "testing"

func TestTuningPresetClampsToBounds(t *testing.T) {
	cur := domainTuningResource{MinTimeoutSeconds: 1, MaxTimeoutSeconds: 30, MaxStreamSeconds: 600}
	ws, err := tuningPreset("websocket", cur)
	if err != nil || !ws.WebSocket || ws.StreamTimeoutSeconds != 600 || ws.StreamCloseDelaySeconds != 30 || ws.Compression {
		t.Fatalf("websocket preset = %+v, %v", ws, err)
	}
	api, _ := tuningPreset("api", cur)
	if !api.Compression || api.ResponseHeaderTimeoutSeconds != 30 {
		t.Fatalf("api preset = %+v", api)
	}
	if _, err := tuningPreset("turbo", cur); err == nil {
		t.Fatal("unknown preset must fail")
	}
	req := ws
	applyTuningFlags(&req, map[string]bool{"websocket": true}, false, false, false, 0, 0, 0, 0)
	if req.WebSocket || req.StreamTimeoutSeconds != 0 || req.StreamCloseDelaySeconds != 0 {
		t.Fatalf("--websocket=false must drop stream values: %+v", req)
	}
}

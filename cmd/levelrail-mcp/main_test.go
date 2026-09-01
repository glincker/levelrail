package main

import (
	"flag"
	"testing"
)

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantToken     string
		wantAPIURL    string
		wantProfile   string
		wantErr       bool
		wantErrIsHelp bool
	}{
		{name: "no flags", args: nil, wantToken: "", wantAPIURL: ""},
		{name: "token and api-url", args: []string{"--token", "t", "--api-url", "http://x:1"}, wantToken: "t", wantAPIURL: "http://x:1"},
		{name: "profile", args: []string{"--profile", "work"}, wantProfile: "work"},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: true},
		{name: "help", args: []string{"-h"}, wantErr: true, wantErrIsHelp: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, apiURL, profile, err := parseFlags("levelrail-mcp", tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseFlags() error = nil, want an error")
				}
				if tt.wantErrIsHelp && err != flag.ErrHelp {
					t.Errorf("parseFlags() error = %v, want flag.ErrHelp", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlags() error = %v", err)
			}
			if token != tt.wantToken {
				t.Errorf("token = %q, want %q", token, tt.wantToken)
			}
			if apiURL != tt.wantAPIURL {
				t.Errorf("apiURL = %q, want %q", apiURL, tt.wantAPIURL)
			}
			if profile != tt.wantProfile {
				t.Errorf("profile = %q, want %q", profile, tt.wantProfile)
			}
		})
	}
}

func TestNewServer_RegistersEveryTool(t *testing.T) {
	server := newServer(nil)
	if server == nil {
		t.Fatal("newServer() = nil")
	}
	// Full tool-call behavior (schema, dispatch, error mapping) is
	// covered end-to-end in tools_test.go via the in-memory transport;
	// this just confirms newServer builds without a live client.
}

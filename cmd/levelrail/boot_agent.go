package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/GLINCKER/levelrail/internal/agent"
)

// A regenerated CA would invalidate every enrolled agent's certificate.
func loadOrGenerateAgentCA(dataDir string) (*agent.CA, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return nil, err
	}
	certPath := filepath.Join(dataDir, agentCACertFilename)
	keyPath := filepath.Join(dataDir, agentCAKeyFilename)

	certPEM, certErr := os.ReadFile(certPath) //nolint:gosec // operator-controlled data directory path, not user input
	keyPEM, keyErr := os.ReadFile(keyPath)    //nolint:gosec // same
	if certErr == nil && keyErr == nil {
		return agent.LoadCA(certPEM, keyPEM)
	}

	ca, err := agent.GenerateCA()
	if err != nil {
		return nil, fmt.Errorf("generate agent CA: %w", err)
	}
	if err := os.WriteFile(certPath, ca.CertPEM(), 0o644); err != nil { //nolint:gosec // certificate is not secret
		return nil, fmt.Errorf("persist agent CA certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, ca.KeyPEM(), 0o600); err != nil { //nolint:gosec // operator-controlled data directory path, not user input
		return nil, fmt.Errorf("persist agent CA private key: %w", err)
	}
	return ca, nil
}

// agentAddr reads APP_AGENT_ADDR, defaulting to defaultAgentAddr.
func agentAddr() string {
	addr := os.Getenv("APP_AGENT_ADDR")
	if addr == "" {
		addr = defaultAgentAddr
	}
	return addr
}

// Must match the gRPC server's TLS certificate or every agent's verification fails.
// Defaults to 127.0.0.1, correct only for single-machine testing.
func agentAdvertiseHost() string {
	host := os.Getenv("APP_AGENT_ADVERTISE_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	return host
}

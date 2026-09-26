package main

import (
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
)

// agentCertServerOptions reads the agent certificate settings (ADR 021):
// APP_AGENT_CERT_VALIDITY, APP_AGENT_CERT_RENEW_GRACE and
// APP_AGENT_REQUIRE_CSR.
func agentCertServerOptions() []agent.Option {
	requireCSR, _ := strconv.ParseBool(os.Getenv("APP_AGENT_REQUIRE_CSR"))
	return []agent.Option{
		agent.WithCertValidity(envDuration("APP_AGENT_CERT_VALIDITY", agent.DefaultClientCertValidity)),
		agent.WithRenewGrace(envDuration("APP_AGENT_CERT_RENEW_GRACE", agent.DefaultCertRenewGrace)),
		agent.WithRequireCSR(requireCSR),
	}
}

// nodeCertThresholds reads APP_NODE_CERT_EXPIRY_WARNING and
// APP_NODE_CERT_EXPIRY_CRITICAL, shared by the node API and the
// node_cert_expiring alert so both agree.
func nodeCertThresholds() (warning, critical time.Duration) {
	return envDuration("APP_NODE_CERT_EXPIRY_WARNING", alerting.DefaultNodeCertWarning),
		envDuration("APP_NODE_CERT_EXPIRY_CRITICAL", alerting.DefaultNodeCertCritical)
}

// configureNodeCerts applies the node certificate and agent version
// settings to the API router. APP_AGENT_MIN_VERSION empty disables the
// outdated-agent check.
func configureNodeCerts(rt *api.Router, sessions api.NodeSessionCloser) {
	warning, critical := nodeCertThresholds()
	for _, opt := range []api.Option{
		api.WithNodeCertThresholds(warning, critical),
		api.WithMinAgentVersion(os.Getenv("APP_AGENT_MIN_VERSION")),
		api.WithNodeSessionCloser(sessions),
	} {
		opt(rt)
	}
}

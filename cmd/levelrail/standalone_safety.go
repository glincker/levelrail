package main

import (
	"github.com/GLINCKER/levelrail/internal/api"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/standalone"
)

// standaloneSafety lists this instance's own listeners: a standalone domain
// may target the dashboard or agent port only after explicit confirmation,
// and never the ingress admin API.
func standaloneSafety() api.StandaloneSafety {
	ports := []standalone.ControlPort{
		{Port: ingressPortFromAddr(ingressreconcile.DefaultAdminListen), Label: "ingress admin API", AlwaysBlock: true},
		{Port: ingressPortFromAddr(httpAddr()), Label: "dashboard and API"},
		{Port: ingressPortFromAddr(agentAddr()), Label: "agent gRPC listener"},
	}
	return api.StandaloneSafety{
		ControlPorts: ports,
		IngressPorts: []int{ingressPortFromAddr(ingressHTTPAddr()), ingressPortFromAddr(ingressHTTPSAddr())},
	}
}

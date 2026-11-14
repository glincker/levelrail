package main

import (
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/standalone"
)

// standaloneSafety lists this instance's own listeners: a standalone domain
// may target the dashboard or agent port only after explicit confirmation,
// and never the ingress admin API.
func standaloneSafety() api.StandaloneSafety {
	ports := []standalone.ControlPort{
		{Port: ingressPortFromAddr(httpAddr()), Label: "dashboard and API"},
		{Port: ingressPortFromAddr(agentAddr()), Label: "agent gRPC listener"},
	}
	if admin := ingressAdminDialAddr(); admin != "" {
		ports = append([]standalone.ControlPort{{Port: ingressPortFromAddr(admin), Label: "ingress admin API", AlwaysBlock: true}}, ports...)
	}
	return api.StandaloneSafety{
		ControlPorts: ports,
		IngressPorts: []int{ingressPortFromAddr(ingressHTTPAddr()), ingressPortFromAddr(ingressHTTPSAddr())},
	}
}

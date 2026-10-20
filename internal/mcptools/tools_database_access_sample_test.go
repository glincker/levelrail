package mcptools

import (
	"encoding/json"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func sampleNetwork() apiclient.DatabaseNetwork {
	var n apiclient.DatabaseNetwork
	raw := `{"database":"main","running":true,"internal":{"host":"db-main","port":5432},
	"networks":[{"name":"bridge","kind":"default-bridge"}],
	"clients":[{"app":"web","via":"env","in_scope":true},{"app":"worker","via":"env","in_scope":false}],
	"verdict":{"level":"private","text":"Private: only 1 app can reach it."},
	"rules":{"active":true,"allow":[{"source":"10.0.0.0/8"}],"extra":["203.0.113.9/32"]},
	"scope":{"current":"project"},"tls":{"required":true,"state":"required"}}`
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		panic(err)
	}
	return n
}

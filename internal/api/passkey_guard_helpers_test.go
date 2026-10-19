package api

import "github.com/GLINCKER/levelrail/internal/authengine"

func authengineTestPasskey(aaguid string, synced *bool) authengine.Passkey {
	return authengine.Passkey{ID: "pk_1", Label: "Test", AAGUID: aaguid, Synced: synced}
}

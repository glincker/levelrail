package authengine

import (
	"testing"
	"time"
)

func FuzzParseSAMLIdPMetadata(f *testing.F) {
	f.Add([]byte(`<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="x"></EntityDescriptor>`))
	f.Add([]byte(`<!DOCTYPE a [<!ENTITY b "c">]><a>&b;</a>`))
	f.Add([]byte("<a><a><a>"))
	f.Add([]byte(""))
	f.Fuzz(func(_ *testing.T, raw []byte) {
		_, _ = ParseSAMLIdPMetadata(raw, time.Unix(1_800_000_000, 0))
	})
}

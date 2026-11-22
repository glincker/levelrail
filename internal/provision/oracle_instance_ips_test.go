package provision

import (
	"context"
	"testing"
)

func TestOracleListResolvesIPsThroughVnics(t *testing.T) {
	o, f := newInstOracle(t, flexDoc)
	f.att = `[{"instanceId":"` + tInstance + `","vnicId":"ocid1.vnic.oc1.phx.ipsa","lifecycleState":"ATTACHED"}]`
	f.vnics = map[string]string{"ocid1.vnic.oc1.phx.ipsa": `{"publicIp":"150.1.2.3","privateIp":"10.0.0.5"}`}
	rows, err := o.ListInstances(context.Background(), InstanceScope{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows %v err %v", rows, err)
	}
	if rows[0].PublicIPv4 != "150.1.2.3" || rows[0].PrivateIPv4 != "10.0.0.5" || rows[0].IPNote != "" {
		t.Fatalf("row %+v", rows[0])
	}
}

func TestOracleListDegradesWithoutNetworkRead(t *testing.T) {
	o, f := newInstOracle(t, flexDoc)
	f.attStatus = 404
	rows, err := o.ListInstances(context.Background(), InstanceScope{})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows %v err %v", rows, err)
	}
	if rows[0].PublicIPv4 != "" || rows[0].IPNote != OracleNetworkNote {
		t.Fatalf("row %+v", rows[0])
	}
}

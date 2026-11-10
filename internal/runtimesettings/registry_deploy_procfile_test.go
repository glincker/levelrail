package runtimesettings

import "testing"

func TestProcfileDefaultPortSetting(t *testing.T) {
	var def *Definition
	for _, d := range Definitions() {
		if d.Key == KeyDeployProcfileDefaultPort {
			def = &d
			break
		}
	}
	if def == nil {
		t.Fatal("deploy.procfile_default_port is not registered")
	}
	if def.Default != "8080" || def.Apply != ApplyLive || def.Category != CategoryDeploys {
		t.Fatalf("unexpected definition: %+v", def)
	}
	for in, wantErr := range map[string]bool{"1": false, "65535": false, "0": true, "65536": true, "x": true} {
		if err := def.CheckValue(in); (err != nil) != wantErr {
			t.Errorf("%q: err = %v, wantErr %v", in, err, wantErr)
		}
	}
}

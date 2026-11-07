package runtimesettings

import "testing"

func TestOutboundUTMSetting(t *testing.T) {
	svc := New(newFakeStore(), envOf(nil))
	ctx := t.Context()
	if !svc.Bool(ctx, KeyUIOutboundUTM) {
		t.Fatal("default must be true")
	}
	got, err := svc.Set(ctx, KeyUIOutboundUTM, raw(`false`), "alice")
	if err != nil || string(got.Value) != "false" || got.Source != SourceSaved {
		t.Fatalf("Set = %+v, %v", got, err)
	}
	if svc.Bool(ctx, KeyUIOutboundUTM) {
		t.Fatal("saved false not applied live")
	}
	got, err = svc.Reset(ctx, KeyUIOutboundUTM)
	if err != nil || string(got.Value) != "true" || got.Source != SourceDefault {
		t.Fatalf("Reset = %+v, %v", got, err)
	}
	if got.Env != "APP_UI_OUTBOUND_UTM" {
		t.Fatalf("env = %q", got.Env)
	}
}

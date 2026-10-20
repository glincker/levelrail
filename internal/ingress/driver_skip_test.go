package ingress

import (
	"context"
	"errors"
	"testing"
)

func TestDriverApply_SkipsUnchangedConfig(t *testing.T) {
	orig := loadConfig
	t.Cleanup(func() { loadConfig = orig })
	var loads int
	var failNext bool
	loadConfig = func([]byte, bool) error {
		loads++
		if failNext {
			failNext = false
			return errors.New("boom")
		}
		return nil
	}

	d := New(nil)
	a := &Config{Apps: Apps{}}
	b := &Config{Admin: &AdminConfig{Listen: "127.0.0.1:2999"}, Apps: Apps{}}
	steps := []struct {
		name      string
		cfg       *Config
		fail      bool
		wantLoads int
		wantErr   bool
	}{
		{name: "first apply loads", cfg: a, wantLoads: 1},
		{name: "identical config is skipped", cfg: a, wantLoads: 1},
		{name: "changed config loads", cfg: b, wantLoads: 2},
		{name: "failed load", cfg: a, fail: true, wantLoads: 3, wantErr: true},
		{name: "retry after failure loads again", cfg: a, wantLoads: 4},
	}
	for _, s := range steps {
		failNext = s.fail
		err := d.Apply(context.Background(), s.cfg)
		if (err != nil) != s.wantErr {
			t.Fatalf("%s: error = %v, want error %v", s.name, err, s.wantErr)
		}
		if loads != s.wantLoads {
			t.Fatalf("%s: loads = %d, want %d", s.name, loads, s.wantLoads)
		}
	}
}

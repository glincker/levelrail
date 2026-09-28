package api

import (
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestDoctorCheckContainerHardening(t *testing.T) {
	tests := []struct {
		name       string
		cfg        docker.HardeningConfig
		err        error
		wantStatus string
		wantSubstr string
	}{
		{"warn reports what enforce would set", docker.HardeningConfig{Mode: docker.HardeningWarn, PidsLimit: 4096}, nil, doctorStatusWarn, "enforce would set cap_drop=ALL"},
		{"enforce is ok", docker.HardeningConfig{Mode: docker.HardeningEnforce, PidsLimit: 4096}, nil, doctorStatusOK, "enforce:"},
		{"off warns", docker.HardeningConfig{Mode: docker.HardeningOff}, nil, doctorStatusWarn, "off:"},
		{"bad config warns", docker.HardeningConfig{Mode: docker.HardeningWarn}, errors.New("bad value"), doctorStatusWarn, "bad value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := doctorCheckContainerHardening(tt.cfg, tt.err)
			if got.Status != tt.wantStatus || !strings.Contains(got.Message, tt.wantSubstr) {
				t.Errorf("got %+v, want status %q containing %q", got, tt.wantStatus, tt.wantSubstr)
			}
		})
	}
}

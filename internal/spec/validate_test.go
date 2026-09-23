package spec

import (
	"testing"
)

func TestService_EffectiveReplicas(t *testing.T) {
	tests := []struct {
		name     string
		replicas int
		want     int
	}{
		{"default fallback", 0, DefaultReplicas},
		{"explicit replicas", 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &Service{
				Replicas: tt.replicas,
			}
			if got := svc.EffectiveReplicas(); got != tt.want {
				t.Errorf("EffectiveReplicas() = %d, want %d", got, tt.want)
			}
		})
	}
}

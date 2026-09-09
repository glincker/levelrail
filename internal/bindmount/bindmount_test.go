package bindmount

import "testing"

func TestValidateHostPath(t *testing.T) {
	tests := []struct {
		name     string
		hostPath string
		wantErr  bool
	}{
		{name: "valid absolute path", hostPath: "/srv/myapp/data", wantErr: false},
		{name: "relative path rejected", hostPath: "data", wantErr: true},
		{name: "root rejected", hostPath: "/", wantErr: true},
		{name: "exact forbidden path rejected", hostPath: "/etc", wantErr: true},
		{name: "forbidden path prefix rejected", hostPath: "/etc/passwd", wantErr: true},
		{name: "docker socket rejected", hostPath: "/var/run/docker.sock", wantErr: true},
		{name: "var run prefix rejected", hostPath: "/var/run/anything", wantErr: true},
		{name: "similar but distinct path allowed", hostPath: "/etcetera/data", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostPath(tt.hostPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHostPath(%q) error = %v, wantErr %v", tt.hostPath, err, tt.wantErr)
			}
		})
	}
}

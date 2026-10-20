package docker

import "testing"

func TestIsProjectNamedVolume(t *testing.T) {
	tests := []struct {
		name string
		vol  string
		want bool
	}{
		{name: "app service volume", vol: "app-web-data", want: true},
		{name: "compose app volume", vol: "app-myapp-web-data", want: true},
		{name: "database data volume", vol: "db-main-data", want: true},
		{name: "database certs volume", vol: "db-main-certs", want: true},
		{name: "anonymous hex volume is not project-named", vol: anonID64, want: false},
		{name: "operator-created volume is not project-named", vol: "my-custom-volume", want: false},
		{name: "empty string is not project-named", vol: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isProjectNamedVolume(tt.vol); got != tt.want {
				t.Errorf("isProjectNamedVolume(%q) = %v, want %v", tt.vol, got, tt.want)
			}
		})
	}
}

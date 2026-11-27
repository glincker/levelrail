package dockerguard

import "testing"

func TestGrantableDevice(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/dev/dri", true},
		{"/dev/dri/renderD128", true},
		{"/dev/dri/card0", true},
		{"/dev/ttyUSB0", true},
		{"/dev/ttyACM12", true},
		{"/dev/video0", true},
		{"/dev/mem", false},
		{"/dev/kmem", false},
		{"/dev/sda", false},
		{"/dev/sda1", false},
		{"/dev/nvme0n1", false},
		{"/dev/disk/by-id/x", false},
		{"/dev/fuse", false},
		{"/dev/kvm", false},
		{"/dev/ttyUSB", false},
		{"/dev/ttyUSB*", false},
		{"/dev/video*", false},
		{"/dev/dri/../mem", false},
		{"/dev/dri/..", false},
		{"/dev/../dev/mem", false},
		{"/dev//mem", false},
		{"/dev/dri//card0", false},
		{"/dev/video0/../mem", false},
		{"/dev/ttyUSB0 ", false},
		{"dev/video0", false},
		{"/tmp/dev/video0", false},
		{"", false},
	}
	for _, c := range cases {
		if got := GrantableDevice(c.path); got != c.want {
			t.Errorf("GrantableDevice(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

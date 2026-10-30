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
		{name: "run symlink target rejected", hostPath: "/run/docker.sock", wantErr: true},
		{name: "ancestor of docker dir rejected", hostPath: "/var/lib", wantErr: true},
		{name: "ancestor var rejected", hostPath: "/var", wantErr: true},
		{name: "control plane data dir rejected", hostPath: "/var/lib/levelrail-data/secrets", wantErr: true},
		{name: "dev rejected", hostPath: "/dev", wantErr: true},
		{name: "traversal normalised", hostPath: "/srv/../etc/shadow", wantErr: true},
		{name: "var www allowed", hostPath: "/var/www/site", wantErr: false},
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

func TestValidateHostPath_ExecutableDirsAndConfiguredDataDirs(t *testing.T) {
	t.Setenv("APP_DATA_DIR", "/srv/levelrail-state")
	for _, p := range []string{
		"/usr/local/bin", "/usr/local/bin/tools", "/usr/bin", "/bin", "/sbin", "/lib", "/lib64",
		"/var/spool/cron", "/srv/levelrail-state", "/srv/levelrail-state/secrets",
	} {
		if err := ValidateHostPath(p); err == nil {
			t.Errorf("ValidateHostPath(%q) = nil, want a protected path error", p)
		}
	}
	for _, p := range []string{"/srv/media", "/opt/app-data", "/home/deploy/uploads", "/mnt/nas"} {
		if err := ValidateHostPath(p); err != nil {
			t.Errorf("ValidateHostPath(%q) = %v, want nil", p, err)
		}
	}
}

func TestValidateHostPath_CredentialDirsAndHomeRoots(t *testing.T) {
	for _, p := range []string{
		"/home/ubuntu/.ssh", "/home/ubuntu/.ssh/authorized_keys", "/srv/x/.aws", "/opt/stack/.kube/config",
		"/home", "/home/ubuntu", "/var/log", "/var/log/auth.log", "/usr/local/etc/app",
	} {
		if err := ValidateHostPath(p); err == nil {
			t.Errorf("ValidateHostPath(%q) = nil, want rejected", p)
		}
	}
	for _, p := range []string{"/home/deploy/uploads", "/opt/app-data", "/srv/media", "/home/deploy/sshfiles"} {
		if err := ValidateHostPath(p); err != nil {
			t.Errorf("ValidateHostPath(%q) = %v, want nil", p, err)
		}
	}
}

func TestValidateHostPath_AllowedRoots(t *testing.T) {
	t.Setenv(EnvAllowedRoots, "/srv/apps:/mnt/nas")
	for _, p := range []string{"/srv/apps/site", "/srv/apps", "/mnt/nas/media"} {
		if err := ValidateHostPath(p); err != nil {
			t.Errorf("ValidateHostPath(%q) = %v, want nil", p, err)
		}
	}
	for _, p := range []string{"/opt/app-data", "/srv/appsx/data", "/srv/apps/../other"} {
		if err := ValidateHostPath(p); err == nil {
			t.Errorf("ValidateHostPath(%q) = nil, want outside allowed roots", p)
		}
	}
	t.Setenv(EnvAllowedRoots, "/")
	if err := ValidateHostPath("/srv/media"); err == nil {
		t.Error("a root of / must not allow everything")
	}
}

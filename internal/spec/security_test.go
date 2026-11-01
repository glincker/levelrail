package spec

import "testing"

func TestValidateSecurity(t *testing.T) {
	on, off := true, false
	tests := []struct {
		name    string
		svc     Service
		wantErr bool
	}{
		{"unset", Service{}, false},
		{"true", Service{Security: &Security{ReadOnlyRootfs: &on}}, false},
		{"true with files", Service{Security: &Security{ReadOnlyRootfs: &on}, Files: []File{{Path: "/etc/a", Content: "x"}}}, true},
		{"false with files", Service{Security: &Security{ReadOnlyRootfs: &off}, Files: []File{{Path: "/etc/a", Content: "x"}}}, false},
		{"true on static", Service{Build: Build{Type: BuildStatic}, Security: &Security{ReadOnlyRootfs: &on}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.svc.validateSecurity("web"); (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

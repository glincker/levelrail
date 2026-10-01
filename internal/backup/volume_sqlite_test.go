package backup

import "testing"

func TestValidateSqlitePath(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"app/data.db", false},
		{"data.db", false},
		{"", true},
		{"   ", true},
		{"/etc/passwd", true},
		{"../escape.db", true},
		{"app/../../escape.db", true},
	}
	for _, tt := range tests {
		err := ValidateSqlitePath(tt.path)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateSqlitePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
		}
	}
}

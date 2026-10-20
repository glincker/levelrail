package bindaddr

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "empty defaults to private", value: "", want: "127.0.0.1"},
		{name: "private shorthand", value: "private", want: "127.0.0.1"},
		{name: "public shorthand", value: "public", want: "0.0.0.0"},
		{name: "literal ipv4", value: "10.0.0.5", want: "10.0.0.5"},
		{name: "literal wildcard ipv4 passthrough", value: "0.0.0.0", want: "0.0.0.0"},
		{name: "literal ipv6", value: "::1", want: "::1"},
		{name: "garbage rejected", value: "  public", wantErr: true},
		{name: "typo rejected", value: "publik", wantErr: true},
		{name: "not-quite-empty whitespace rejected", value: " ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.value)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Resolve(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	if err := Validate("private"); err != nil {
		t.Errorf("Validate(private) = %v, want nil", err)
	}
	if err := Validate("not-a-value"); err == nil {
		t.Error("Validate(not-a-value) = nil, want error")
	}
}

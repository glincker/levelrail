package compose

import (
	"testing"
)

func TestParsePort(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		// Valid cases
		{name: "standard port", input: "80", want: 80, wantErr: false},
		{name: "min valid port", input: "1", want: 1, wantErr: false},
		{name: "max valid port", input: "65535", want: 65535, wantErr: false},

		// Out of range cases
		{name: "zero port", input: "0", want: 0, wantErr: true},
		{name: "negative port", input: "-1", want: 0, wantErr: true},
		{name: "above max port", input: "65536", want: 0, wantErr: true},

		// Invalid format cases
		{name: "empty string", input: "", want: 0, wantErr: true},
		{name: "letters", input: "abc", want: 0, wantErr: true},
		{name: "decimal point", input: "80.80", want: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePort(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePort() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parsePort() got = %v, want %v", got, tt.want)
			}
		})
	}
}

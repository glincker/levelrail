package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWriteZoneError(t *testing.T) {
	tests := []struct {
		name  string
		zones []string
		want  int
	}{
		{"with zones", []string{"a.com", "b.org"}, 2},
		{"no zones omits field", nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeZoneError(rec, "no zone", tc.zones)
			var body struct {
				Error string   `json:"error"`
				Code  string   `json:"code"`
				Zones []string `json:"zones"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 400 || body.Error != "no zone" || body.Code == "" || len(body.Zones) != tc.want {
				t.Fatalf("got %d %+v", rec.Code, body)
			}
		})
	}
}

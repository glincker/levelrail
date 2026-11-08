package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/diskhistory"
)

func TestRun_SystemDiskHistory(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
		var samples []diskhistory.Sample
		for i := 0; i < 10; i++ {
			samples = append(samples, diskhistory.Sample{At: base.Add(time.Duration(i) * time.Hour), FreeBytes: 50 << 30, TotalBytes: 100 << 30})
		}
		d := 12.0
		_ = json.NewEncoder(w).Encode(apiclient.DiskHistory{Samples: samples, RetentionDays: 14, Forecast: apiclient.DiskForecast{DaysUntilFull: &d, BasisDays: 7}})
	}))
	defer srv.Close()

	var out, errOut bytes.Buffer
	args := []string{"system", "disk", "history", "--days", "3", "--rows", "4", "--api-url", srv.URL}
	if got := run("levelrail-cli-test", args, &out, &errOut, envMap()); got != exitOK {
		t.Fatalf("exit %d %q", got, errOut.String())
	}
	if gotQuery != "days=3" {
		t.Errorf("query %q", gotQuery)
	}
	if !strings.Contains(out.String(), "full in about 12 days") || !strings.Contains(out.String(), "showing 4 of 10") || !strings.Contains(out.String(), "2026-11-01 09:00:00") {
		t.Errorf("table: %q", out.String())
	}

	out.Reset()
	if got := run("levelrail-cli-test", []string{"system", "disk", "history", "--json", "--api-url", srv.URL}, &out, &errOut, envMap()); got != exitOK {
		t.Fatalf("json exit %d %q", got, errOut.String())
	}
	var res apiclient.DiskHistory
	if err := json.Unmarshal(out.Bytes(), &res); err != nil || len(res.Samples) != 10 {
		t.Errorf("json: %v %q", err, out.String())
	}

	if got := run("levelrail-cli-test", []string{"system", "disk", "history", "--days", "-1", "--api-url", srv.URL}, &out, &errOut, envMap()); got != exitValidation {
		t.Errorf("negative days exit %d", got)
	}
}

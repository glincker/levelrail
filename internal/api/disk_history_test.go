package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/diskhistory"
)

type fakeDiskHistory struct{ samples []diskhistory.Sample }

func (f fakeDiskHistory) Since(_ context.Context, since time.Time) ([]diskhistory.Sample, error) {
	var out []diskhistory.Sample
	for _, s := range f.samples {
		if !s.At.Before(since) {
			out = append(out, s)
		}
	}
	return out, nil
}

func TestDiskHistoryEndpoint(t *testing.T) {
	rt, db := newTestRouter(t)
	const gib = int64(1) << 30
	now := time.Now().UTC()
	var samples []diskhistory.Sample
	for i := 0; i < 10; i++ {
		used := 10*gib + int64(i)*gib
		samples = append(samples, diskhistory.Sample{At: now.Add(-time.Duration(9-i) * 24 * time.Hour), TotalBytes: 100 * gib, FreeBytes: 100*gib - used})
	}
	rt.SetDiskHistory(fakeDiskHistory{samples: samples})
	seedBoundTestToken(t, db, "tok_dh_read", "plain-dh-read", AbilityRead)
	seedBoundTestToken(t, db, "tok_dh_deploy", "plain-dh-deploy", AbilityDeploy)

	rec := bearerGet(rt, "/api/v1/system/disk/history?days=5", "plain-dh-read")
	if rec.Code != http.StatusOK {
		t.Fatalf("read token status = %d, body %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Samples  []diskhistory.Sample `json:"samples"`
		Forecast struct {
			DaysUntilFull *float64 `json:"days_until_full"`
			BasisDays     int      `json:"basis_days"`
		} `json:"forecast"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Samples) != 5 {
		t.Fatalf("samples = %d, want 5", len(got.Samples))
	}
	if got.Forecast.DaysUntilFull == nil || got.Forecast.BasisDays != 10 {
		t.Fatalf("forecast = %+v, want a value over 10 days", got.Forecast)
	}

	if code := bearerGet(rt, "/api/v1/system/disk/history", "plain-dh-deploy").Code; code != http.StatusForbidden {
		t.Fatalf("token without read status = %d, want 403", code)
	}
}

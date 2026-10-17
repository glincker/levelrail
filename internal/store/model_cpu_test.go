package store

import (
	"context"
	"errors"
	"testing"
)

func TestModelCPUFieldsRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	m := testModel("c", "")
	m.Compute, m.CPUMilli, m.MemoryMiB = ComputeCPU, 1500, 3072
	m.WeightsRepo, m.WeightsFile, m.WeightsSHA256, m.LicenseAck = "o/n-GGUF", "n.gguf", "ab", true
	if err := db.SaveModel(ctx, m); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetModel(ctx, "c")
	if err != nil {
		t.Fatal(err)
	}
	if got.Compute != ComputeCPU || got.CPUMilli != 1500 || got.MemoryMiB != 3072 || got.WeightsFile != "n.gguf" || !got.LicenseAck || got.WeightsSHA256 != "ab" {
		t.Fatalf("got %+v", got)
	}
	if err := db.SetModelDownload(ctx, "c", 10, 100); err != nil {
		t.Fatal(err)
	}
	if err := db.SetModelBenchmark(ctx, "c", `{"gen":1}`); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetModel(ctx, "c")
	if got.DownloadDoneBytes != 10 || got.DownloadTotalBytes != 100 || got.BenchmarkJSON != `{"gen":1}` {
		t.Fatalf("progress/benchmark = %+v", got)
	}
	if err := db.SetModelDownload(ctx, "missing", 1, 2); !errors.Is(err, ErrModelNotFound) {
		t.Errorf("err = %v", err)
	}
}

func TestModelDefaultsToGPU(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveModel(ctx, testModel("g", "")); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetModel(ctx, "g")
	if got.Compute != ComputeGPU || got.CPUMilli != 0 || got.BenchmarkJSON != "" {
		t.Fatalf("got %+v", got)
	}
}

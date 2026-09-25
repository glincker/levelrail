#!/bin/bash
echo "Original index: database_name, started_at DESC"
echo "Original backup_history_test.go doesn't have a benchmark."
cat << 'EOG' > internal/store/backup_history_benchmark_test.go
package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func BenchmarkListAllBackupHistory(b *testing.T) {
	db := mustOpenTestDB(b)
	ctx := context.Background()

	// Seed 10k rows
	for i := 0; i < 10000; i++ {
		db.SaveBackupHistory(ctx, BackupHistory{
			DatabaseName: "mydb",
			TargetID:     "t-1",
			ObjectKey:    fmt.Sprintf("backup-%d", i),
			Status:       "success",
			StartedAt:    time.Now().Add(-time.Duration(i) * time.Minute),
			FinishedAt:   time.Now().Add(-time.Duration(i) * time.Minute),
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := db.ListAllBackupHistory(ctx, 50, nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
EOG
go test -bench=BenchmarkListAllBackupHistory -benchmem ./internal/store

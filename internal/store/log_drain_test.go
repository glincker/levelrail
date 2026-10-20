package store

import (
	"context"
	"errors"
	"testing"
)

func TestUpdateServiceLogDrain(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		drain *LogDrain
	}{
		{
			name:  "http sink",
			drain: &LogDrain{Type: LogDrainHTTP, Target: "https://collector.example.com/ingest", Enabled: true},
		},
		{
			name:  "syslog sink",
			drain: &LogDrain{Type: LogDrainSyslog, Target: "udp://logs.example.com:514", Enabled: true},
		},
		{
			name:  "disabled drain still persists target",
			drain: &LogDrain{Type: LogDrainHTTP, Target: "https://collector.example.com/ingest", Enabled: false},
		},
		{
			name:  "nil clears any existing drain",
			drain: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
				t.Fatalf("SaveDesiredService() error = %v", err)
			}

			if err := db.UpdateServiceLogDrain(ctx, "web", tt.drain); err != nil {
				t.Fatalf("UpdateServiceLogDrain() error = %v", err)
			}

			got, err := db.GetDesiredService(ctx, "web")
			if err != nil {
				t.Fatalf("GetDesiredService() error = %v", err)
			}
			if tt.drain == nil {
				if got.LogDrain != nil {
					t.Errorf("LogDrain = %+v, want nil", got.LogDrain)
				}
				return
			}
			if got.LogDrain == nil {
				t.Fatalf("LogDrain = nil, want %+v", tt.drain)
			}
			if *got.LogDrain != *tt.drain {
				t.Errorf("LogDrain = %+v, want %+v", got.LogDrain, tt.drain)
			}
		})
	}
}

func TestUpdateServiceLogDrain_RoundTripThenClear(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.UpdateServiceLogDrain(ctx, "web", &LogDrain{Type: LogDrainHTTP, Target: "https://a.example.com", Enabled: true}); err != nil {
		t.Fatalf("UpdateServiceLogDrain(set) error = %v", err)
	}
	if err := db.UpdateServiceLogDrain(ctx, "web", nil); err != nil {
		t.Fatalf("UpdateServiceLogDrain(clear) error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.LogDrain != nil {
		t.Errorf("LogDrain = %+v, want nil after clearing", got.LogDrain)
	}
}

func TestUpdateServiceLogDrain_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdateServiceLogDrain(context.Background(), "nonexistent", &LogDrain{Type: LogDrainHTTP, Target: "https://a.example.com", Enabled: true})
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("UpdateServiceLogDrain() error = %v, want ErrServiceNotFound", err)
	}
}

// TestSaveDesiredService_DoesNotTouchLogDrain proves an ordinary redeploy
// (SaveDesiredService's full-record-replace path) never clears an
// already-configured drain, the same invariant
// TestUpdateServiceStorageTarget's sibling tests establish for
// storage_target_id.
func TestSaveDesiredService_DoesNotTouchLogDrain(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	drain := &LogDrain{Type: LogDrainSyslog, Target: "udp://logs.example.com:514", Enabled: true}
	if err := db.UpdateServiceLogDrain(ctx, "web", drain); err != nil {
		t.Fatalf("UpdateServiceLogDrain() error = %v", err)
	}

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v2", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService(redeploy) error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.LogDrain == nil || *got.LogDrain != *drain {
		t.Errorf("LogDrain = %+v after redeploy, want unchanged %+v", got.LogDrain, drain)
	}
}

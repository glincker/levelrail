package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type mockWebhookDeliveryStore struct {
	deletedOlderThan time.Time
	returnedDeleted  int64
	returnedError    error
	callCount        int
}

func (m *mockWebhookDeliveryStore) DeleteWebhookDeliveriesOlderThan(_ context.Context, cutoff time.Time) (int64, error) {
	m.deletedOlderThan = cutoff
	m.callCount++
	return m.returnedDeleted, m.returnedError
}

func (m *mockWebhookDeliveryStore) SaveWebhookDelivery(_ context.Context, _ store.WebhookDelivery) error {
	return nil
}

func (m *mockWebhookDeliveryStore) GetWebhookDelivery(_ context.Context, _ string) (*store.WebhookDelivery, error) {
	return nil, nil
}

func (m *mockWebhookDeliveryStore) ListWebhookDeliveries(_ context.Context, _ string, _ int, _ *time.Time) ([]store.WebhookDelivery, error) {
	return nil, nil
}

func TestPurgeOldWebhookDeliveries(t *testing.T) {
	mock := &mockWebhookDeliveryStore{returnedDeleted: 42}

	rt := &Router{
		webhookDeliveries:        mock,
		webhookDeliveryRetention: 24 * time.Hour,
	}

	before := time.Now().UTC()

	n, err := rt.PurgeOldWebhookDeliveries(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 42 {
		t.Errorf("deleted = %d, want 42", n)
	}

	after := time.Now().UTC()
	expectedMin := before.Add(-24 * time.Hour)
	expectedMax := after.Add(-24 * time.Hour)
	if mock.deletedOlderThan.Before(expectedMin) || mock.deletedOlderThan.After(expectedMax) {
		t.Errorf("cutoff = %v, want between %v and %v", mock.deletedOlderThan, expectedMin, expectedMax)
	}
}

func TestPurgeOldWebhookDeliveries_DefaultRetention(t *testing.T) {
	mock := &mockWebhookDeliveryStore{returnedDeleted: 7}
	rt := &Router{webhookDeliveries: mock}

	before := time.Now().UTC()

	n, err := rt.PurgeOldWebhookDeliveries(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 7 {
		t.Errorf("deleted = %d, want 7", n)
	}

	after := time.Now().UTC()
	expectedMin := before.Add(-defaultWebhookDeliveryRetention)
	expectedMax := after.Add(-defaultWebhookDeliveryRetention)
	if mock.deletedOlderThan.Before(expectedMin) || mock.deletedOlderThan.After(expectedMax) {
		t.Errorf("cutoff = %v, want between %v and %v", mock.deletedOlderThan, expectedMin, expectedMax)
	}
}

func TestRunWebhookDeliverySweeper(t *testing.T) {
	mock := &mockWebhookDeliveryStore{returnedDeleted: 1}
	rt := &Router{webhookDeliveries: mock, logger: discardLogger()}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- rt.RunWebhookDeliverySweeper(ctx, 5*time.Millisecond)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	if err := <-errCh; err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if mock.callCount == 0 {
		t.Error("expected DeleteWebhookDeliveriesOlderThan to be called at least once")
	}
}

func TestRunWebhookDeliverySweeper_Error(t *testing.T) {
	mock := &mockWebhookDeliveryStore{returnedError: errors.New("mock error")}
	rt := &Router{webhookDeliveries: mock, logger: discardLogger()}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- rt.RunWebhookDeliverySweeper(ctx, 5*time.Millisecond)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	if err := <-errCh; err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if mock.callCount == 0 {
		t.Error("expected DeleteWebhookDeliveriesOlderThan to be called at least once")
	}
}

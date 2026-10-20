package store

import (
	"context"
	"testing"
)

func TestListLoadBalancerRows_OrderedAndJoined(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, name := range []string{"zeta", "alpha", "plain"} {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: name, Image: "img:v1", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"zeta", "alpha"} {
		if err := db.SetServiceLoadBalancer(ctx, name, `{"algorithm":"least_conn"}`); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.ListLoadBalancerRows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Service != "alpha" || rows[1].Service != "zeta" || rows[0].UpdatedAt == "" {
		t.Fatalf("rows = %+v, want alpha then zeta only", rows)
	}
}

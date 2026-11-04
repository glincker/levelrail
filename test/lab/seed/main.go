// Command seed inserts lab nodes straight into the store, so scheduler rules
// can be exercised without agents. It is a test aid and is never shipped.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "lab seed:", err)
		os.Exit(1)
	}
}

func run() error {
	dir := flag.String("data-dir", "", "lab data dir that holds levelrail.db")
	count := flag.Int("nodes", 0, "how many nodes to seed")
	zones := flag.String("zones", "a,b", "comma separated zone names, assigned round robin")
	flag.Parse()
	if *dir == "" || *count < 1 {
		return errors.New("--data-dir and --nodes (at least 1) are required")
	}
	zoneList := strings.Split(*zones, ",")
	if err := os.MkdirAll(*dir, 0o750); err != nil {
		return err
	}
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(*dir, "levelrail.db"))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	now := time.Now().UTC()
	for i := 1; i <= *count; i++ {
		id := fmt.Sprintf("lab-node-%d", i)
		zone := strings.TrimSpace(zoneList[(i-1)%len(zoneList)])
		n := store.Node{
			ID: id, Name: id, Address: fmt.Sprintf("10.99.0.%d", i), Status: store.NodeStatusOnline,
			JoinedAt: &now, LastSeenAt: &now, AcceptsAppWorkloads: true, CreatedAt: now, UpdatedAt: now,
		}
		if err := db.SaveNode(ctx, n); err != nil && !errors.Is(err, store.ErrNodeNameTaken) {
			return err
		}
		if err := db.UpdateNodeFailureDomain(ctx, id, "lab", "lab-region", zone); err != nil {
			return err
		}
		if err := db.SetNodeLabels(ctx, id, map[string]string{"pool": "lab"}); err != nil {
			return err
		}
	}
	fmt.Printf("seeded %d nodes\n", *count)
	return nil
}

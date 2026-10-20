package backup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// scratchSearchSpec gives a search engine's drill container the boot script
// that loads a pending snapshot, keyed with a random throwaway API key.
func scratchSearchSpec(engine string, spec *docker.ContainerSpec) error {
	if !store.IsSearchEngine(engine) {
		return nil
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("generate scratch api key: %w", err)
	}
	database.ScratchSearchSpec(engine, hex.EncodeToString(buf), spec)
	return nil
}

// contentSummaryReader reads the counts recorded when a backup was taken.
type contentSummaryReader interface {
	GetBackupContentSummary(ctx context.Context, backupHistoryID string) (string, error)
}

// checkSearchContent reads the restored engine's counts and, when the
// backup recorded stable counts, requires an exact match.
func (r *DrillRunner) checkSearchContent(ctx context.Context, container, engine, backupID string) (int64, error) {
	got, err := ReadSearchContent(ctx, r.Runtime, engine, container)
	if err != nil {
		return 0, fmt.Errorf("query restored %s: %w", engine, err)
	}
	reader, ok := r.Store.(contentSummaryReader)
	if !ok {
		return int64(len(got)), nil
	}
	recorded, err := reader.GetBackupContentSummary(ctx, backupID)
	if err != nil {
		return 0, fmt.Errorf("read recorded content counts: %w", err)
	}
	if recorded == "" {
		return int64(len(got)), nil
	}
	want, err := DecodeSearchContent(recorded)
	if err != nil {
		return 0, err
	}
	if diff := want.Diff(got); diff != "" {
		return 0, fmt.Errorf("restored content does not match the counts recorded at backup time: %s", diff)
	}
	return int64(len(got)), nil
}

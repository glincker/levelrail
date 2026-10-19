package backup

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestClassifyDrillFailure(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want string
	}{
		{"credentials", "check object: StatusCode: 403, AccessDenied", DrillFailureCredentials},
		{"disk", "write: no space left on device", DrillFailureDisk},
		{"missing object", `object "x" is missing from the bucket`, DrillFailureObject},
		{"checksum", "checksum of the stored object does not match the one recorded", DrillFailureObject},
		{"network", "dial tcp 10.0.0.1:443: connect: connection refused", DrillFailureDestination},
		{"unknown", "psql: syntax error", DrillFailureOther},
		{"empty", "", DrillFailureOther},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyDrillFailure(tc.err)
			if got.Kind != tc.want || got.Guidance == "" {
				t.Fatalf("ClassifyDrillFailure(%q) = %+v, want kind %s", tc.err, got, tc.want)
			}
		})
	}
}

type removeTrackingRuntime struct {
	*fakeExecRuntime
	created, removed []string
}

func (r *removeTrackingRuntime) Create(_ context.Context, spec docker.ContainerSpec) (string, error) {
	r.created = append(r.created, spec.Name)
	return "scratch-1", nil
}

func (r *removeTrackingRuntime) Remove(_ context.Context, id string, _ bool) error {
	r.removed = append(r.removed, id)
	return nil
}

type noopDBRestorer struct{}

func (noopDBRestorer) Restore(_ context.Context, _, _ string, dump io.Reader) error {
	_, err := io.Copy(io.Discard, dump)
	return err
}

func TestDrillDatabase_ScratchContainerRemovedWhenVerificationFails(t *testing.T) {
	tests := []struct {
		name    string
		dump    string
		wantErr string
	}{
		{"truncated dump fails content check", "CREATE TABLE t (id int);\n", "truncated"},
		{"complete dump passes", "CREATE TABLE t (id int);\n-- PostgreSQL database dump complete\n", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hist, objs := newMemHistory(), newMemObjects()
			objs.objects["db/main.sql"] = []byte(tc.dump)
			hist.rows["bkh_1"] = store.BackupHistory{
				ID: "bkh_1", ResourceKind: store.BackupResourceKindDatabase, DatabaseName: "main", TargetID: "bkt_test",
				ObjectKey: "db/main.sql", Status: store.BackupStatusSucceeded, SizeBytes: int64(len(tc.dump)),
			}
			rt := &removeTrackingRuntime{fakeExecRuntime: &fakeExecRuntime{content: "3\n"}}
			r := &DrillRunner{
				Store: hist, Secrets: newTestSecrets(), Downloader: objs, Prober: objs,
				Runtime: rt, DBRestorer: noopDBRestorer{},
				DBInfo:   func(context.Context, string) (string, string, error) { return store.EnginePostgres, "16", nil },
				ImageFor: func(string, string) string { return "postgres:16" },
			}
			d, err := r.RunDrill(context.Background(), "bkd_1", "bkh_1", store.DrillTriggerManual)
			if tc.wantErr == "" {
				if err != nil || d.Status != store.DrillStatusPassed {
					t.Fatalf("want pass, got %v %+v", err, d)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) || d.Status != store.DrillStatusFailed {
				t.Fatalf("want failure containing %q, got %v %+v", tc.wantErr, err, d)
			}
			if len(rt.created) != 1 || len(rt.removed) != 1 || rt.removed[0] != "scratch-1" {
				t.Fatalf("scratch container leaked: created %v removed %v", rt.created, rt.removed)
			}
			if hist.drills["bkd_1"].FinishedAt == "" {
				t.Fatal("drill row was not finished")
			}
		})
	}
}

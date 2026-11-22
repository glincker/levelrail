package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleAppRestoreToNode(t *testing.T) {
	t.Parallel()
	const path = "/api/v1/apps/web/restore-to-node"
	newEnv := func(t *testing.T, p *fakeBackupProtection) (*Router, *http.Cookie) {
		rt, db := newProtectionRouter(t, p, nil)
		cookie := loginTestSession(t, rt, db)
		seedServiceWithVolume(t, db)
		seedVolumeBackup(t, db, "bkh_1", store.BackupStatusSucceeded)
		return rt, cookie
	}

	t.Run("restores the succeeded volume backup onto the node", func(t *testing.T) {
		p := &fakeBackupProtection{known: true, restores: make(chan [3]string, 1)}
		rt, cookie := newEnv(t, p)
		rec := do(t, rt, cookie, http.MethodPost, path, `{"node_id":"n2"}`)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
		}
		var out appRestoreToNodeResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Items) != 1 {
			t.Fatalf("body = %s (%v)", rec.Body.String(), err)
		}
		if it := out.Items[0]; it.Status != restoreItemStarted || it.BackupID != "bkh_1" || !strings.HasPrefix(it.NewName, "clone-web-") {
			t.Fatalf("item = %+v", it)
		}
		select {
		case got := <-p.restores:
			if got[2] != out.Items[0].BackupID {
				t.Fatalf("call = %v", got)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("RestoreVolumeTo was not called")
		}
	})
	t.Run("dry run creates nothing", func(t *testing.T) {
		p := &fakeBackupProtection{known: true, restores: make(chan [3]string, 1)}
		rt, cookie := newEnv(t, p)
		rec := do(t, rt, cookie, http.MethodPost, path, `{"node_id":"n2","dry_run":true}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
		}
		select {
		case <-p.restores:
			t.Fatal("dry run restored a volume")
		case <-time.After(100 * time.Millisecond):
		}
	})
	t.Run("node is required", func(t *testing.T) {
		p := &fakeBackupProtection{known: true, restores: make(chan [3]string, 1)}
		rt, cookie := newEnv(t, p)
		if rec := do(t, rt, cookie, http.MethodPost, path, `{}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}

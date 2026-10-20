package api

import (
	"net/http"
	"strings"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Restore plan step states.
const (
	restoreStepDone    = "done"
	restoreStepTodo    = "todo"
	restoreStepBlocked = "blocked"
)

type controlPlaneRestoreStep struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

type controlPlaneRestoreBackup struct {
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"created_at"`
	SizeBytes int64     `json:"size_bytes"`
}

type controlPlaneRestorePlan struct {
	Ready      bool                       `json:"ready"`
	Backup     *controlPlaneRestoreBackup `json:"backup,omitempty"`
	InstallID  string                     `json:"install_id,omitempty"`
	Source     string                     `json:"source,omitempty"`
	Endpoint   string                     `json:"endpoint,omitempty"`
	Command    string                     `json:"command,omitempty"`
	Steps      []controlPlaneRestoreStep  `json:"steps"`
	Warnings   []string                   `json:"warnings"`
	BinaryName string                     `json:"binary_name"`
}

// handleGetControlPlaneRestorePlan handles GET /api/v1/system/control-plane-dr/restore-plan:
// the ordered recovery steps for this install with the newest complete off-box backup filled in. Restore itself runs offline because it replaces the live database.
func (rt *Router) handleGetControlPlaneRestorePlan(w http.ResponseWriter, r *http.Request) {
	dr, ok := rt.cpDROrNotImplemented(w)
	if !ok {
		return
	}
	st, err := dr.Status(r.Context())
	if err != nil {
		rt.writeCPDRError(w, "restore plan status", err)
		return
	}
	list, err := dr.ListRemote(r.Context())
	if err != nil {
		rt.writeCPDRError(w, "restore plan list", err)
		return
	}

	plan := controlPlaneRestorePlan{InstallID: st.InstallID, BinaryName: rt.brand.BinaryName, Warnings: []string{}}
	for _, b := range list {
		if !b.Complete {
			continue
		}
		if plan.Backup == nil || b.CreatedAt.After(plan.Backup.CreatedAt) {
			plan.Backup = &controlPlaneRestoreBackup{Key: b.Key, CreatedAt: b.CreatedAt, SizeBytes: b.SizeBytes}
		}
	}

	backupState := restoreStepBlocked
	if plan.Backup != nil {
		backupState = restoreStepDone
		rt.fillRestoreSource(r, st.TargetID, &plan)
	}
	escrowState := restoreStepTodo
	if st.Checklist.EscrowAcknowledged {
		escrowState = restoreStepDone
	} else {
		plan.Warnings = append(plan.Warnings, "escrow_not_acknowledged")
	}
	if !st.Checklist.DrillPassed {
		plan.Warnings = append(plan.Warnings, "drill_not_passed")
	}
	plan.Steps = []controlPlaneRestoreStep{
		{ID: "backup_available", State: backupState},
		{ID: "identity_ready", State: restoreStepTodo},
		{ID: "master_key_escrow", State: escrowState},
		{ID: "stop_control_plane", State: restoreStepTodo},
		{ID: "run_restore", State: restoreStepTodo},
		{ID: "start_and_verify", State: restoreStepTodo},
	}
	plan.Ready = plan.Backup != nil && st.Checklist.RecipientSet
	writeJSON(w, http.StatusOK, plan)
}

func (rt *Router) fillRestoreSource(r *http.Request, targetID string, plan *controlPlaneRestorePlan) {
	if targetID == "" || plan.Backup == nil {
		return
	}
	t, err := rt.backupTargets.GetBackupTarget(r.Context(), targetID)
	if err != nil || t.Bucket == "" {
		return
	}
	plan.Source = "s3://" + t.Bucket + "/" + strings.TrimPrefix(plan.Backup.Key, "/")
	cmd := rt.brand.BinaryName + " restore --from " + plan.Source + " --identity <private-key-file>"
	if t.Endpoint != "" && !strings.HasPrefix(t.Endpoint, store.AgentEndpointScheme) {
		plan.Endpoint = t.Endpoint
		cmd += " --endpoint " + t.Endpoint
	}
	plan.Command = cmd
}

type controlPlaneDRKeypair struct {
	Recipient string `json:"recipient"`
	Identity  string `json:"identity"`
}

// handleGenerateControlPlaneDRKeypair handles POST /api/v1/system/control-plane-dr/keys: mints an age keypair
// for backup recipients. The private half is returned once and never stored or logged.
func (rt *Router) handleGenerateControlPlaneDRKeypair(w http.ResponseWriter, _ *http.Request) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		rt.logger.Error("api: generate backup recipient keypair failed")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, controlPlaneDRKeypair{Recipient: id.Recipient().String(), Identity: id.String()})
}

package api

import "fmt"

// doctorCheckDatabaseAudit warns when explorer audit events were dropped or rows failed to write since start.
func (rt *Router) doctorCheckDatabaseAudit() doctorCheckResource {
	const code, name = "database_audit_health", "Database audit trail"
	if rt.dbAudit == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "The database explorer audit is not running."}
	}
	dropped, failed := rt.dbAudit.Dropped(), rt.dbAudit.Failed()
	if dropped == 0 && failed == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "The database explorer audit is recording without loss."}
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message: fmt.Sprintf("%d audit events were dropped and %d rows failed to write since start", dropped, failed),
		Fix:     "Raise databases.audit_queue_size, check the control plane disk and database health, then restart to reset the counters.",
	}
}

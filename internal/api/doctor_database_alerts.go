package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/alerting"
)

// doctorCheckDatabaseAlerts is one row listing the databases that are breaching a health alert rule right now.
func (rt *Router) doctorCheckDatabaseAlerts(ctx context.Context) doctorCheckResource {
	const code, name = "database_alerts", "Database health alerts"
	inputs, err := rt.DatabaseAlertInputs(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: "Could not read database health: " + err.Error()}
	}
	names := alerting.DatabaseAlertingNames(inputs, rt.DatabaseAlertPolicy(ctx), time.Now())
	if len(names) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "No database is breaching a health alert."}
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message: fmt.Sprintf("%d database(s) breaching a health alert: %s", len(names), strings.Join(names, ", ")),
		Fix:     "Open each database's monitoring page for the value and the next step, or tune alerts.database.* in settings.",
	}
}

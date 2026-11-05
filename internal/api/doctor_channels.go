package api

import (
	"context"
	"fmt"
)

// doctorCheckNotifyReachable warns when no notification channel and no
// email provider exist, so an outage would only be seen by looking.
func (rt *Router) doctorCheckNotifyReachable(ctx context.Context) doctorCheckResource {
	const code, name = "notify_reachable", "Outage notifications"
	channels := 0
	if rt.notificationChannels != nil {
		rows, err := rt.notificationChannels.ListNotificationChannels(ctx)
		if err != nil {
			return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read notification channels: %s", err)}
		}
		channels = len(rows)
	}
	if channels > 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%d notification channel(s) configured", channels)}
	}
	providers := 0
	if rt.emailProviders != nil {
		rows, err := rt.emailProviders.ListEmailProviders(ctx)
		if err != nil {
			return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read email providers: %s", err)}
		}
		for _, p := range rows {
			if p.Enabled {
				providers++
			}
		}
	}
	if providers > 0 || rt.emailReady(ctx) {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "an email provider is configured"}
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message: "no notification channel or email provider is configured, so nobody is told when something breaks. Add a channel under Settings, then Notifications, or run the command below",
		Fix:     rt.cliName() + " channels create --name NAME --kind KIND --notify-url URL",
	}
}

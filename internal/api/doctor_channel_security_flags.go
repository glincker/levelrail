package api

import (
	"context"
	"fmt"
	"strings"
)

// doctorCheckNotificationChannelSecurityFlags lists channels that receive
// instance sign-in notices, because a channel records no creator and setting
// the flag was open to non-root callers before it became root only.
func (rt *Router) doctorCheckNotificationChannelSecurityFlags(ctx context.Context) doctorCheckResource {
	const code, name = "notification_channel_security_flags", "Sign-in notice channels"
	if rt.notificationChannels == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "notification channels are not configured"}
	}
	channels, err := rt.notificationChannels.ListNotificationChannels(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not inspect notification channels: %s", err)}
	}
	var names []string
	for _, c := range channels {
		if c.Enabled && c.ProjectID == "" && (c.NotifyDeviceLogin || c.NotifyDeviceLoginExpired) {
			names = append(names, c.Name)
		}
	}
	if len(names) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "no channel receives sign-in notices"}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn,
		Message:  fmt.Sprintf("%d channel(s) receive sign-in notices with account emails and IP addresses, confirm each destination is trusted: %s", len(names), strings.Join(names, ", ")),
		DocsPath: "/security#notification-channel-secrets"}
}

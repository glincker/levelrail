package api

import (
	"context"
	"fmt"
)

// plaintextChannelCounter is the optional NotificationChannels surface
// the doctor reads; *alerting.DB satisfies it.
type plaintextChannelCounter interface {
	CountPlaintextNotificationChannels(ctx context.Context) (int, error)
}

// doctorCheckNotificationChannelSecrets warns while any notification
// channel credential is still stored in plaintext.
func (rt *Router) doctorCheckNotificationChannelSecrets(ctx context.Context) doctorCheckResource {
	const code, name = "notification_channel_secrets", "Notification channel secrets"
	counter, ok := rt.notificationChannels.(plaintextChannelCounter)
	if !ok {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "notification channels are not configured"}
	}
	n, err := counter.CountPlaintextNotificationChannels(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not inspect notification channels: %s", err)}
	}
	if n == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "every channel credential is encrypted at rest"}
	}
	msg := fmt.Sprintf("%d notification channel(s) store their credential in plaintext, restart to encrypt them", n)
	if rt.masterKeyRotator == nil {
		msg = fmt.Sprintf("%d notification channel(s) store their credential in plaintext because no master key is configured", n)
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: msg, DocsPath: "/security#notification-channel-secrets"}
}
